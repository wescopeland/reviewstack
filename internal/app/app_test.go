package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wescopeland/reviewstack/internal/app"
	"github.com/wescopeland/reviewstack/internal/cli"
	"github.com/wescopeland/reviewstack/internal/memory"
)

func TestHeadlessFakeRun(t *testing.T) {
	if os.Getenv("REVIEWSTACK_INTEGRATION") == "" {
		t.Skip("set REVIEWSTACK_INTEGRATION=1 to run")
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	opts := &cli.Options{
		PR:            99,
		Base:          "HEAD",
		Target:        "HEAD",
		FakeReviewers: true,
		NoTUI:         true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.New(opts).Run(ctx); err != nil {
		t.Fatal(err)
	}

	runs, err := filepath.Glob(filepath.Join(dir, ".reviewstack", "runs", "*-pr-99"))
	if err != nil || len(runs) == 0 {
		t.Fatalf("expected run dir, glob err=%v runs=%v", err, runs)
	}
	runDir := runs[0]
	for _, rel := range []string{
		"raw/claude-aesthetic.md",
		"logs/claude-thermo.err",
		"final-review.md",
	} {
		if _, err := os.Stat(filepath.Join(runDir, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}

	memPath := filepath.Join(dir, ".reviewstack", "memory", "pr-99.json")
	if _, err := os.Stat(memPath); err != nil {
		t.Fatalf("expected memory index at %s: %v", memPath, err)
	}
}

func TestSynthesisOnlyFailsWithoutPromptFallback(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	runDir := seedSynthesisRun(t, dir)
	configPath := writeSynthesisConfig(t, dir, `
  command: sh
  args:
    - -c
    - exit 7
`)

	opts := &cli.Options{
		SynthesizeOnly: runDir,
		ConfigPath:     configPath,
	}
	err = app.New(opts).Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "synthesis") {
		t.Fatalf("expected synthesis error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(runDir, "final-review.md")); !os.IsNotExist(err) {
		t.Fatalf("final report should not be prompt fallback, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(runDir, "synthesis-prompt.md")); err != nil {
		t.Fatalf("expected prompt artifact: %v", err)
	}
}

func TestSynthesisOnlyPromotesTempFinalReport(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	runDir := seedSynthesisRun(t, dir)
	configPath := writeSynthesisConfig(t, dir, `
  command: sh
  args:
    - -c
    - 'printf "# final\n" > "$1"'
    - reviewstack
    - "{{final}}"
`)

	opts := &cli.Options{
		SynthesizeOnly: runDir,
		ConfigPath:     configPath,
	}
	if err := app.New(opts).Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	final, err := os.ReadFile(filepath.Join(runDir, "final-review.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(final) != "# final\n" {
		t.Fatalf("final = %q", final)
	}
	if _, err := os.Stat(filepath.Join(runDir, "final-review.md.tmp")); !os.IsNotExist(err) {
		t.Fatalf("temp report should be promoted, stat err=%v", err)
	}
}

func TestSynthesisOnlyCapturesCommandOutput(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	runDir := seedSynthesisRun(t, dir)
	configPath := writeSynthesisConfig(t, dir, `
  command: sh
  args:
    - -c
    - 'printf "stdout text\n"; printf "stderr text\n" >&2; printf "# final\n" > "$1"'
    - reviewstack
    - "{{final}}"
`)

	opts := &cli.Options{
		SynthesizeOnly: runDir,
		ConfigPath:     configPath,
	}
	if err := app.New(opts).Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	stdout, err := os.ReadFile(filepath.Join(runDir, "logs", "synthesis.out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stdout) != "stdout text\n" {
		t.Fatalf("stdout log = %q", stdout)
	}
	stderr, err := os.ReadFile(filepath.Join(runDir, "logs", "synthesis.err"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stderr) != "stderr text\n" {
		t.Fatalf("stderr log = %q", stderr)
	}
}

func TestHeadlessFakeRereviewRun(t *testing.T) {
	if os.Getenv("REVIEWSTACK_INTEGRATION") == "" {
		t.Skip("set REVIEWSTACK_INTEGRATION=1 to run")
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	// Seed a prior PR run in memory.
	priorRun := filepath.Join(dir, ".reviewstack", "runs", "20260501-120000-pr-99")
	inputs := filepath.Join(priorRun, "inputs")
	if err := os.MkdirAll(inputs, 0o755); err != nil {
		t.Fatal(err)
	}
	priorFinal := "# Prior review\n- check nil pointer"
	if err := os.WriteFile(filepath.Join(priorRun, "final-review.md"), []byte(priorFinal), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputs, "context.json"), []byte(`{"pr":99,"title":"Test PR"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := memory.RecordFromRunDir(priorRun, 99, false)
	if err := memory.IndexRun(dir, rec); err != nil {
		t.Fatal(err)
	}

	opts := &cli.Options{
		PR:            99,
		Base:          "HEAD",
		Target:        "HEAD",
		FakeReviewers: true,
		NoTUI:         true,
		Rereview:      true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.New(opts).Run(ctx); err != nil {
		t.Fatal(err)
	}

	runs, err := filepath.Glob(filepath.Join(dir, ".reviewstack", "runs", "*-pr-99"))
	if err != nil || len(runs) < 2 {
		t.Fatalf("expected second run dir, glob err=%v runs=%v", err, runs)
	}

	var rereviewDir string
	for _, r := range runs {
		if r != priorRun {
			rereviewDir = r
			break
		}
	}
	if rereviewDir == "" {
		t.Fatal("missing re-review run dir")
	}

	ctxPath := filepath.Join(rereviewDir, "inputs", "rereview-context.json")
	data, err := os.ReadFile(ctxPath)
	if err != nil {
		t.Fatalf("missing rereview context: %v", err)
	}
	if !strings.Contains(string(data), "reviewstack-tester") {
		t.Fatalf("unexpected rereview context: %s", data)
	}

	final, err := os.ReadFile(filepath.Join(rereviewDir, "final-review.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(final), "Re-review report") {
		t.Fatalf("final report missing heading: %s", final)
	}
}

func seedSynthesisRun(t *testing.T, dir string) string {
	t.Helper()

	runDir := filepath.Join(dir, ".reviewstack", "runs", "existing")
	for _, rel := range []string{"inputs", "raw", "logs"} {
		if err := os.MkdirAll(filepath.Join(runDir, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(runDir, "inputs", "context.json"), []byte(`{"mode":"test"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "raw", "demo.md"), []byte("finding"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "logs", "demo.err"), []byte("log"), 0o644); err != nil {
		t.Fatal(err)
	}
	return runDir
}

func writeSynthesisConfig(t *testing.T, dir string, synthesis string) string {
	t.Helper()

	configPath := filepath.Join(dir, ".reviewstack", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `reviewers:
  - id: demo
    command: sh
    args: ["-c", "true"]
synthesis:
` + synthesis
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return configPath
}
