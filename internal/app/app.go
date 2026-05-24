package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wescopeland/reviewstack/internal/cli"
	"github.com/wescopeland/reviewstack/internal/config"
	"github.com/wescopeland/reviewstack/internal/fake"
	"github.com/wescopeland/reviewstack/internal/inputs"
	"github.com/wescopeland/reviewstack/internal/memory"
	"github.com/wescopeland/reviewstack/internal/rereview"
	"github.com/wescopeland/reviewstack/internal/reviewer"
	"github.com/wescopeland/reviewstack/internal/run"
	"github.com/wescopeland/reviewstack/internal/synth"
	"github.com/wescopeland/reviewstack/internal/tools"
	"github.com/wescopeland/reviewstack/internal/tui"
)

type App struct {
	opts      *cli.Options
	cfg       *config.Config
	workspace string
}

func New(opts *cli.Options) *App {
	return &App{opts: opts}
}

func (a *App) Run(ctx context.Context) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	a.workspace = wd

	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	a.cfg = cfg

	if a.opts.SynthesizeOnly != "" {
		return a.runSynthesisOnly(ctx)
	}

	reviewers, err := cfg.FilterReviewers(a.opts.Only)
	if err != nil {
		return err
	}
	ids := reviewerIDs(reviewers)

	if !a.opts.FakeReviewers {
		cmds := make([]string, 0, len(reviewers)+1)
		for _, r := range reviewers {
			cmds = append(cmds, r.Command)
		}
		synthCmd := ""
		if !a.opts.SkipSynthesis {
			cmds = append(cmds, cfg.Synthesis.Command)
			synthCmd = cfg.Synthesis.Command
		}
		reqs := tools.Discover(cmds, synthCmd, a.opts.PR > 0)
		if err := tools.Validate(reqs); err != nil && !a.opts.DryRun {
			return err
		}
	}

	layout, err := run.Create(a.workspace, run.NamingInput{
		Now:   time.Now(),
		PR:    a.opts.PR,
		Label: inputs.RunLabel(a.opts.Mode, a.opts.Base),
	}, ids)
	if err != nil {
		return err
	}

	if !a.opts.DryRun {
		if err := inputs.Gather(layout.Inputs, inputs.Options{
			PR:          a.opts.PR,
			PRURL:       a.opts.PRURL,
			Base:        a.opts.Base,
			Target:      a.opts.Target,
			Uncommitted: a.opts.Uncommitted,
			Mode:        a.opts.Mode,
		}); err != nil {
			return fmt.Errorf("gather inputs: %w", err)
		}
		if a.opts.Rereview {
			if err := a.gatherRereviewContext(layout); err != nil {
				return err
			}
		}
	}

	paths := make(map[string]reviewer.Paths, len(ids))
	for id, p := range layout.Reviewers {
		paths[id] = reviewer.Paths{Raw: p.Raw, Log: p.Log}
	}

	if a.opts.DryRun {
		fmt.Printf("Run dir: %s\n", layout.Root)
		fmt.Printf("Workspace: %s\n", a.workspace)
		rc := a.reviewerRunContext(layout)
		for _, r := range reviewers {
			expanded := runContextFrom(rc).ExpandArgs(r.Args)
			fmt.Printf("  %s: %s %v\n", r.ID, r.Command, expanded)
		}
		return nil
	}

	eventCh := make(chan reviewer.Status, 32)
	mgr := reviewer.NewManager(func(st reviewer.Status) {
		select {
		case eventCh <- st:
		default:
		}
	})
	mgr.Init(ids, paths)

	reviewerByID := make(map[string]config.Reviewer, len(reviewers))
	for _, r := range reviewers {
		reviewerByID[r.ID] = r
	}

	model := tui.New(tui.Config{
		PR:             a.opts.PR,
		Subject:        a.tuiSubject(),
		Base:           a.opts.Base,
		RunDir:         layout.Root,
		Reviewers:      reviewers,
		Manager:        mgr,
		Events:         eventCh,
		AutoSynthesize: !a.opts.SkipSynthesis,
		OnRerun: func(id string) error {
			r, ok := reviewerByID[id]
			if !ok {
				return fmt.Errorf("reviewer %q not found", id)
			}
			if err := mgr.Reset(id); err != nil {
				return err
			}
			return mgr.Start(ctx, r, a.reviewerRunContext(layout))
		},
		OnKill: func(id string) error {
			return mgr.Kill(id)
		},
		OnSynthesize: func(ctx context.Context) error {
			return a.synthesize(ctx, layout, reviewers)
		},
	})

	rc := a.reviewerRunContext(layout)
	for _, r := range reviewers {
		if err := mgr.Start(ctx, r, rc); err != nil {
			fmt.Fprintf(os.Stderr, "warning: start %s: %v\n", r.ID, err)
		}
	}

	if a.opts.NoTUI {
		return a.runHeadless(ctx, mgr, layout, reviewers, eventCh)
	}

	if err := tui.Run(model); err != nil {
		return err
	}

	if a.opts.Open && fileExists(layout.Final) {
		return openFile(layout.Final)
	}
	return nil
}

func (a *App) runHeadless(ctx context.Context, mgr *reviewer.Manager, layout *run.Layout, reviewers []config.Reviewer, eventCh <-chan reviewer.Status) error {
	for {
		if allSettled(mgr.Snapshot()) {
			return a.completeHeadlessRun(ctx, layout, reviewers)
		}
		select {
		case <-ctx.Done():
			mgr.KillAll()
			return ctx.Err()
		case _, ok := <-eventCh:
			if !ok {
				eventCh = nil
			}
		}
	}
}

func (a *App) completeHeadlessRun(ctx context.Context, layout *run.Layout, reviewers []config.Reviewer) error {
	if !a.opts.SkipSynthesis {
		if err := a.synthesize(ctx, layout, reviewers); err != nil {
			return err
		}
	} else if a.opts.PR > 0 {
		a.indexRun(layout)
	}
	fmt.Printf("Run complete: %s\n", layout.Root)
	if fileExists(layout.Final) {
		fmt.Printf("Final report: %s\n", layout.Final)
	}
	if a.opts.Open && fileExists(layout.Final) {
		if err := openFile(layout.Final); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) runSynthesisOnly(ctx context.Context) error {
	runDir := a.opts.SynthesizeOnly
	if a.opts.RunDir != "" {
		runDir = a.opts.RunDir
	}
	cfg := a.cfg
	reviewers := cfg.Reviewers
	if len(a.opts.Only) > 0 {
		var err error
		reviewers, err = cfg.FilterReviewers(a.opts.Only)
		if err != nil {
			return err
		}
	}
	layout, err := run.Open(runDir, reviewerIDs(reviewers))
	if err != nil {
		return err
	}
	return a.synthesize(ctx, layout, reviewers)
}

func (a *App) synthesize(ctx context.Context, layout *run.Layout, reviewers []config.Reviewer) error {
	ids := reviewerIDs(reviewers)
	paths := make(map[string]synth.Paths, len(ids))
	for id, p := range layout.Reviewers {
		paths[id] = synth.Paths{Raw: p.Raw, Log: p.Log}
	}
	content, err := synth.Assemble(synth.Input{
		RunDir:    layout.Root,
		Reviewers: ids,
		Paths:     paths,
		Rereview:  a.opts.Rereview,
	})
	if err != nil {
		return err
	}
	promptPath, err := synth.WritePrompt(layout.Root, content)
	if err != nil {
		return err
	}

	if a.opts.FakeReviewers {
		return a.runFakeSynthesis(ctx, promptPath, layout)
	}

	cmd, finalTmp, err := a.buildSynthesisCommand(ctx, layout)
	if err != nil {
		return err
	}
	if err := a.streamSynthesisIO(cmd, promptPath, layout, finalTmp); err != nil {
		return err
	}
	if err := promoteSynthesisOutput(finalTmp, layout.Final); err != nil {
		return err
	}
	if a.opts.PR > 0 {
		a.indexRun(layout)
	}
	return nil
}

func (a *App) runFakeSynthesis(ctx context.Context, promptPath string, layout *run.Layout) error {
	heading := "Synthesized Review"
	if a.opts.Rereview {
		heading = "Re-review report"
	}
	script := fmt.Sprintf(`
head -n 40 "$1" > "$2"
echo "# %s" >> "$2"
echo "" >> "$2"
echo "Merged findings from all reviewers." >> "$2"
`, heading)
	cmd := exec.CommandContext(ctx, shell, "-c", script, "reviewstack", promptPath, layout.Final)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("fake synthesis: %w: %s", err, string(out))
	}
	if a.opts.PR > 0 {
		a.indexRun(layout)
	}
	return nil
}

func (a *App) buildSynthesisCommand(ctx context.Context, layout *run.Layout) (*exec.Cmd, string, error) {
	rc := a.reviewerRunContext(layout)
	runCtx := runContextFrom(rc)
	finalTmp := layout.Final + ".tmp"
	runCtx.Final = finalTmp
	args := runCtx.ExpandArgs(a.cfg.Synthesis.Args)
	_ = os.Remove(finalTmp)

	cmd := exec.CommandContext(ctx, a.cfg.Synthesis.Command, args...)
	cmd.Dir = rc.Workspace
	if len(a.cfg.Synthesis.Env) > 0 {
		cmd.Env = append(os.Environ(), a.cfg.Synthesis.Env...)
	}
	return cmd, finalTmp, nil
}

func (a *App) streamSynthesisIO(cmd *exec.Cmd, promptPath string, layout *run.Layout, finalTmp string) error {
	if err := os.MkdirAll(layout.Logs, 0o755); err != nil {
		return fmt.Errorf("create synthesis log dir: %w", err)
	}
	stdoutPath := filepath.Join(layout.Logs, "synthesis.out")
	stderrPath := filepath.Join(layout.Logs, "synthesis.err")
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return fmt.Errorf("create synthesis stdout log: %w", err)
	}
	defer func() { _ = stdoutFile.Close() }()
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return fmt.Errorf("create synthesis stderr log: %w", err)
	}
	defer func() { _ = stderrFile.Close() }()
	promptFile, err := os.Open(promptPath)
	if err != nil {
		return err
	}
	defer func() { _ = promptFile.Close() }()
	cmd.Stdin = promptFile
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	if err := cmd.Run(); err != nil {
		_ = os.Remove(finalTmp)
		return fmt.Errorf("synthesis: %w (prompt saved at %s; logs: %s, %s)", err, promptPath, stdoutPath, stderrPath)
	}
	return nil
}

func promoteSynthesisOutput(tmpPath, finalPath string) error {
	info, err := os.Stat(tmpPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("synthesis: command completed without writing %s", finalPath)
		}
		return fmt.Errorf("synthesis: stat output: %w", err)
	}
	if info.Size() == 0 {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("synthesis: command wrote empty report")
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("synthesis: finalize report: %w", err)
	}
	return nil
}

func (a *App) gatherRereviewContext(layout *run.Layout) error {
	if a.opts.PR <= 0 {
		return fmt.Errorf("re-review requires a PR number")
	}
	runs, err := memory.LoadPR(a.workspace, a.opts.PR)
	if err != nil {
		return fmt.Errorf("load review memory: %w", err)
	}
	prior := memory.SelectPriorRun(runs, layout.Root)

	var client rereview.Client
	if a.opts.FakeReviewers {
		client = rereview.FakeClient(a.opts.PR)
	} else {
		client = rereview.NewGHClient()
	}
	return rereview.Gather(layout.Inputs, rereview.Options{
		PR:         a.opts.PR,
		CurrentRun: layout.Root,
		RepoRoot:   a.workspace,
		PriorRun:   prior,
	}, client)
}

func (a *App) indexRun(layout *run.Layout) {
	if a.opts.PR <= 0 {
		return
	}
	rec := memory.RecordFromRunDir(layout.Root, a.opts.PR, a.opts.Rereview)
	if err := memory.IndexRun(a.workspace, rec); err != nil {
		fmt.Fprintf(os.Stderr, "warning: index run memory: %v\n", err)
	}
}

func (a *App) loadConfig() (*config.Config, error) {
	if a.opts.FakeReviewers {
		return fake.Config(), nil
	}
	path := a.opts.ConfigPath
	if path == "" {
		path = config.DefaultConfigPath
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return config.Default(), nil
		}
		return nil, err
	}
	return config.Load(path)
}

func reviewerIDs(reviewers []config.Reviewer) []string {
	ids := make([]string, len(reviewers))
	for i, r := range reviewers {
		ids[i] = r.ID
	}
	return ids
}

func allSettled(states []reviewer.Status) bool {
	if len(states) == 0 {
		return true
	}
	for _, st := range states {
		switch st.State {
		case reviewer.StatePending, reviewer.StateRunning:
			return false
		}
	}
	return true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func openFile(path string) error {
	cmd := exec.Command("open", path)
	return cmd.Run()
}

const shell = "sh"

func (a *App) tuiSubject() string {
	switch a.opts.Mode {
	case cli.ModeUncommitted:
		if label := inputs.RunLabel(a.opts.Mode, a.opts.Base); label != "" {
			return label
		}
		return "uncommitted"
	case cli.ModeBranch:
		return fmt.Sprintf("branch … %s", a.opts.Base)
	case cli.ModePR:
		if a.opts.PR > 0 {
			return fmt.Sprintf("PR %d", a.opts.PR)
		}
	}
	return "review"
}

func (a *App) reviewerRunContext(layout *run.Layout) reviewer.RunContext {
	return reviewer.RunContext{
		Workspace: a.workspace,
		RunDir:    layout.Root,
		Base:      a.opts.Base,
		Target:    a.opts.Target,
		PR:        a.opts.PR,
		Mode:      reviewModeString(a.opts.Mode),
	}
}

func reviewModeString(mode cli.ReviewMode) string {
	switch mode {
	case cli.ModeUncommitted:
		return "uncommitted"
	case cli.ModeBranch:
		return "branch"
	default:
		return "pr"
	}
}

func runContextFrom(rc reviewer.RunContext) run.Context {
	ctx := run.NewContext(rc.Workspace, rc.RunDir, rc.Base, rc.Target, rc.PR)
	if rc.Mode != "" {
		ctx.Mode = rc.Mode
	}
	return ctx
}

// ResolveConfigPath returns the first existing config path.
func ResolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if fileExists(config.DefaultConfigPath) {
		return config.DefaultConfigPath
	}
	home, _ := os.UserHomeDir()
	candidate := filepath.Join(home, ".config", "reviewstack", "config.yaml")
	if fileExists(candidate) {
		return candidate
	}
	return config.DefaultConfigPath
}
