package run

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	RootDir     = ".reviewstack"
	RunsDir     = ".reviewstack/runs"
	InputsDir   = "inputs"
	RawDir      = "raw"
	LogsDir     = "logs"
	FinalReview = "final-review.md"
)

var slugRe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type Layout struct {
	Root      string
	Inputs    string
	Raw       string
	Logs      string
	Final     string
	Reviewers map[string]ReviewerPaths
}

type ReviewerPaths struct {
	Raw string
	Log string
}

type NamingInput struct {
	Now   time.Time
	PR    int
	Label string
}

func DirName(in NamingInput) string {
	ts := in.Now.UTC().Format("20060102-150405")
	suffix := labelSuffix(in)
	if suffix == "" {
		return ts
	}
	return ts + "-" + suffix
}

func labelSuffix(in NamingInput) string {
	if in.PR > 0 {
		return fmt.Sprintf("pr-%d", in.PR)
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		return ""
	}
	return slug(label)
}

func slug(s string) string {
	s = strings.TrimSpace(s)
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "run"
	}
	return s
}

func Create(base string, in NamingInput, reviewerIDs []string) (*Layout, error) {
	baseName := DirName(in)
	runsRoot := filepath.Join(base, RunsDir)
	if err := os.MkdirAll(runsRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create runs dir %s: %w", runsRoot, err)
	}

	var root string
	for i := 0; ; i++ {
		candidate := filepath.Join(runsRoot, baseName)
		if i > 0 {
			candidate = filepath.Join(runsRoot, fmt.Sprintf("%s-%d", baseName, i))
		}
		if err := os.Mkdir(candidate, 0o755); err != nil {
			if os.IsExist(err) {
				continue
			}
			return nil, fmt.Errorf("create run dir %s: %w", candidate, err)
		}
		root = candidate
		break
	}
	layout := &Layout{
		Root:      root,
		Inputs:    filepath.Join(root, InputsDir),
		Raw:       filepath.Join(root, RawDir),
		Logs:      filepath.Join(root, LogsDir),
		Final:     filepath.Join(root, FinalReview),
		Reviewers: make(map[string]ReviewerPaths, len(reviewerIDs)),
	}
	for _, id := range reviewerIDs {
		layout.Reviewers[id] = ReviewerPaths{
			Raw: filepath.Join(layout.Raw, id+".md"),
			Log: filepath.Join(layout.Logs, id+".err"),
		}
	}
	dirs := []string{layout.Inputs, layout.Raw, layout.Logs}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create run dir %s: %w", dir, err)
		}
	}
	return layout, nil
}

func Open(runDir string, reviewerIDs []string) (*Layout, error) {
	info, err := os.Stat(runDir)
	if err != nil {
		return nil, fmt.Errorf("open run dir: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("run path is not a directory: %s", runDir)
	}
	layout := &Layout{
		Root:      runDir,
		Inputs:    filepath.Join(runDir, InputsDir),
		Raw:       filepath.Join(runDir, RawDir),
		Logs:      filepath.Join(runDir, LogsDir),
		Final:     filepath.Join(runDir, FinalReview),
		Reviewers: make(map[string]ReviewerPaths, len(reviewerIDs)),
	}
	for _, id := range reviewerIDs {
		layout.Reviewers[id] = ReviewerPaths{
			Raw: filepath.Join(layout.Raw, id+".md"),
			Log: filepath.Join(layout.Logs, id+".err"),
		}
	}
	return layout, nil
}

func InputPath(layout *Layout, name string) string {
	return filepath.Join(layout.Inputs, name)
}
