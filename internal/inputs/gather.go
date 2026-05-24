package inputs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wescopeland/reviewstack/internal/cli"
)

type Context struct {
	Mode   string `json:"mode,omitempty"`
	PR     int    `json:"pr,omitempty"`
	Base   string `json:"base"`
	Target string `json:"target"`
	Repo   string `json:"repo,omitempty"`
	Title  string `json:"title,omitempty"`
	URL    string `json:"url,omitempty"`
	Branch string `json:"branch,omitempty"`
}

type Options struct {
	PR          int
	PRURL       string
	Base        string
	Target      string
	Uncommitted bool
	Mode        cli.ReviewMode
}

func Gather(dir string, opts Options) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	switch {
	case opts.Uncommitted || opts.Mode == cli.ModeUncommitted:
		return gatherForUncommitted(dir, opts)
	case opts.PR > 0:
		return gatherForPR(dir, opts)
	case opts.Mode == cli.ModeBranch:
		return gatherForBranch(dir, opts)
	default:
		return gatherForBranch(dir, opts)
	}
}

func gatherForUncommitted(dir string, opts Options) error {
	ctx := Context{
		Mode:   "uncommitted",
		Base:   "HEAD",
		Target: "working tree",
		Branch: currentBranch(),
	}
	if err := writeContext(dir, ctx); err != nil {
		return err
	}
	statPath := filepath.Join(dir, "diff-stat.txt")
	diffPath := filepath.Join(dir, "diff.patch")
	return gatherUncommitted(dir, statPath, diffPath)
}

func gatherForPR(dir string, opts Options) error {
	base := opts.Base
	target := opts.Target
	ctx := Context{Base: base, Target: target, URL: opts.PRURL, PR: opts.PR, Mode: "pr"}

	if meta, err := fetchPRMeta(opts.PR); err == nil {
		ctx.Repo = meta.Repo
		ctx.Title = meta.Title
		ctx.URL = meta.URL
		if meta.BaseRef != "" {
			ctx.Base = meta.BaseRef
			base = meta.BaseRef
		}
		if meta.HeadRef != "" {
			ctx.Target = meta.HeadRef
			target = meta.HeadRef
		}
	}

	if err := writeContext(dir, ctx); err != nil {
		return err
	}

	statPath := filepath.Join(dir, "diff-stat.txt")
	diffPath := filepath.Join(dir, "diff.patch")
	if err := writePRDiffStat(statPath, opts.PR, base, target); err != nil {
		return err
	}
	if err := writePRDiff(diffPath, opts.PR, base, target); err != nil {
		return err
	}
	return writePRJSON(dir, opts.PR)
}

func gatherForBranch(dir string, opts Options) error {
	ctx := Context{
		Mode:   "branch",
		Base:   opts.Base,
		Target: opts.Target,
		Branch: currentBranch(),
	}
	if err := writeContext(dir, ctx); err != nil {
		return err
	}

	statPath := filepath.Join(dir, "diff-stat.txt")
	diffPath := filepath.Join(dir, "diff.patch")
	if err := writeCommand(statPath, "git", "diff", "--stat", opts.Base+"..."+opts.Target); err != nil {
		return err
	}
	return writeCommand(diffPath, "git", "diff", opts.Base+"..."+opts.Target)
}

func writeContext(dir string, ctx Context) error {
	if err := writeJSON(filepath.Join(dir, "context.json"), ctx); err != nil {
		return err
	}
	return writeCommand(filepath.Join(dir, "git-status.txt"), "git", "status", "--short")
}

// gatherUncommitted captures staged, unstaged, and untracked changes without mutating
// the user's real git index by copying it to a temporary GIT_INDEX_FILE first.
func gatherUncommitted(dir, statPath, diffPath string) error {
	untracked, err := listUntrackedFiles()
	if err != nil {
		return err
	}

	indexPath, cleanup, err := tempGitIndexCopy()
	if err != nil {
		return err
	}
	defer cleanup()

	env := append(os.Environ(), "GIT_INDEX_FILE="+indexPath)
	if len(untracked) > 0 {
		args := append([]string{"add", "-N", "--"}, untracked...)
		if err := runGitWithEnv(env, args...); err != nil {
			return err
		}
		content := strings.Join(untracked, "\n") + "\n"
		if err := os.WriteFile(filepath.Join(dir, "untracked-files.txt"), []byte(content), 0o644); err != nil {
			return err
		}
	}

	if err := writeCommandWithEnv(env, statPath, "git", "diff", "--stat", "HEAD"); err != nil {
		return err
	}
	return writeCommandWithEnv(env, diffPath, "git", "diff", "HEAD")
}

func tempGitIndexCopy() (path string, cleanup func(), err error) {
	gitDir, err := gitCommonDir()
	if err != nil {
		return "", nil, err
	}
	src := filepath.Join(gitDir, "index")
	tmp, err := os.CreateTemp("", "reviewstack-index-*")
	if err != nil {
		return "", nil, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()

	srcFile, err := os.Open(src)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", nil, fmt.Errorf("open git index: %w", err)
	}
	defer func() { _ = srcFile.Close() }()
	dstFile, err := os.Create(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", nil, err
	}
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		_ = dstFile.Close()
		_ = os.Remove(tmpPath)
		return "", nil, err
	}
	if err := dstFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", nil, err
	}
	return tmpPath, func() { _ = os.Remove(tmpPath) }, nil
}

func gitCommonDir() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --git-common-dir: %w", err)
	}
	dir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(dir) {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(wd, dir)
	}
	return dir, nil
}

func listUntrackedFiles() ([]string, error) {
	out, err := exec.Command("git", "ls-files", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files --others: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines, nil
}

func runGitWithEnv(env []string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, msg)
	}
	return nil
}

func writeCommandWithEnv(env []string, path, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
	}
	return os.WriteFile(path, stdout.Bytes(), 0o644)
}

func writePRJSON(dir string, pr int) error {
	if err := writeCommand(filepath.Join(dir, "pr.json"), "gh", "pr", "view", fmt.Sprint(pr), "--json", "number,title,url,baseRefName,headRefName,author,files"); err != nil {
		return writeJSON(filepath.Join(dir, "pr.json"), map[string]any{
			"number": pr,
			"note":   "gh pr view failed",
			"error":  err.Error(),
		})
	}
	return nil
}

func writePRDiffStat(path string, pr int, base, target string) error {
	err := writePRDiffStatFromGH(path, pr)
	if err == nil {
		return nil
	}
	if gitRefsResolvable(base, target) {
		return writeCommand(path, "git", "diff", "--stat", base+"..."+target)
	}
	return err
}

func writePRDiff(path string, pr int, base, target string) error {
	err := writeCommand(path, "gh", "pr", "diff", fmt.Sprint(pr))
	if err == nil {
		return nil
	}
	if gitRefsResolvable(base, target) {
		return writeCommand(path, "git", "diff", base+"..."+target)
	}
	return err
}

func writePRDiffStatFromGH(path string, pr int) error {
	cmd := exec.Command("gh", "pr", "view", fmt.Sprint(pr), "--json", "files,additions,deletions,changedFiles")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("gh pr view %d: %w: %s", pr, err, msg)
	}

	var payload prDiffStatPayload
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		return fmt.Errorf("parse gh pr view json: %w", err)
	}

	return os.WriteFile(path, []byte(formatPRDiffStat(payload)), 0o644)
}

type prDiffStatPayload struct {
	Files []struct {
		Path      string `json:"path"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
	} `json:"files"`
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
	ChangedFiles int `json:"changedFiles"`
}

func formatPRDiffStat(payload prDiffStatPayload) string {
	var b strings.Builder
	for _, f := range payload.Files {
		total := f.Additions + f.Deletions
		bar := diffStatBar(f.Additions, f.Deletions)
		fmt.Fprintf(&b, " %s | %3d %s\n", f.Path, total, bar)
	}
	fmt.Fprintf(
		&b,
		" %d files changed, %d insertions(+), %d deletions(-)\n",
		payload.ChangedFiles,
		payload.Additions,
		payload.Deletions,
	)
	return b.String()
}

func diffStatBar(additions, deletions int) string {
	total := additions + deletions
	if total == 0 {
		return ""
	}
	const width = 40
	addWidth := additions * width / total
	delWidth := deletions * width / total
	if addWidth+delWidth == 0 {
		addWidth = 1
	}
	return strings.Repeat("+", addWidth) + strings.Repeat("-", delWidth)
}

func gitRefsResolvable(base, target string) bool {
	if strings.TrimSpace(base) == "" || strings.TrimSpace(target) == "" {
		return false
	}
	return gitRefExists(base) && gitRefExists(target)
}

func gitRefExists(ref string) bool {
	return exec.Command("git", "rev-parse", "--verify", ref).Run() == nil
}

func currentBranch() string {
	out, err := exec.Command("git", "branch", "--show-current").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

type prMeta struct {
	Repo    string
	Title   string
	URL     string
	BaseRef string
	HeadRef string
}

func fetchPRMeta(n int) (*prMeta, error) {
	out, err := exec.Command("gh", "pr", "view", fmt.Sprint(n), "--json", "title,url,baseRefName,headRefName,headRepository").Output()
	if err != nil {
		return nil, err
	}
	var payload struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		BaseRefName string `json:"baseRefName"`
		HeadRefName string `json:"headRefName"`
		Head        struct {
			Name string `json:"nameWithOwner"`
		} `json:"headRepository"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, err
	}
	return &prMeta{
		Repo:    payload.Head.Name,
		Title:   payload.Title,
		URL:     payload.URL,
		BaseRef: payload.BaseRefName,
		HeadRef: payload.HeadRefName,
	}, nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func writeCommand(path, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
	}
	return os.WriteFile(path, stdout.Bytes(), 0o644)
}

func RunLabel(mode cli.ReviewMode, base string) string {
	switch mode {
	case cli.ModeUncommitted:
		if branch := currentBranch(); branch != "" {
			return "uncommitted-" + branch
		}
		return "uncommitted"
	case cli.ModeBranch:
		if base != "" {
			return "branch-" + slugBranch(base)
		}
		return "branch-diff"
	default:
		return ""
	}
}

var slugRe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func slugBranch(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "head"
	}
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
