package synth_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wescopeland/reviewstack/internal/synth"
)

func TestAssembleIncludesRawAndLogs(t *testing.T) {
	dir := t.TempDir()
	rawDir := filepath.Join(dir, "raw")
	logDir := filepath.Join(dir, "logs")
	inputDir := filepath.Join(dir, "inputs")
	for _, d := range []string{rawDir, logDir, inputDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(inputDir, "context.json"), []byte(`{"pr":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rawDir, "claude-aesthetic.md"), []byte("# Aesthetic\n- issue A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "claude-aesthetic.err"), []byte("trace: reading diff"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := synth.Assemble(synth.Input{
		RunDir:    dir,
		Reviewers: []string{"claude-aesthetic"},
		Paths: map[string]synth.Paths{
			"claude-aesthetic": {
				Raw: filepath.Join(rawDir, "claude-aesthetic.md"),
				Log: filepath.Join(logDir, "claude-aesthetic.err"),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Aesthetic", "issue A", "trace: reading diff", "Raw output", "Log / stderr"} {
		if !strings.Contains(out, want) {
			t.Fatalf("assembled output missing %q\n%s", want, out)
		}
	}
}

func TestAssembleSanitizesAndTruncatesLogs(t *testing.T) {
	dir := t.TempDir()
	rawDir := filepath.Join(dir, "raw")
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rawDir, "codex-xhigh.md"), []byte("finding"), 0o644); err != nil {
		t.Fatal(err)
	}
	log := "keep start\n" +
		strings.Repeat("x", 300) + "\x00__PAGEZERO\n" +
		strings.Repeat("middle noise\n", 4096) +
		"keep end\n"
	if err := os.WriteFile(filepath.Join(logDir, "codex-xhigh.err"), []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := synth.Assemble(synth.Input{
		RunDir:    dir,
		Reviewers: []string{"codex-xhigh"},
		Paths: map[string]synth.Paths{
			"codex-xhigh": {
				Raw: filepath.Join(rawDir, "codex-xhigh.md"),
				Log: filepath.Join(logDir, "codex-xhigh.err"),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"keep start", "keep end", "omitted"} {
		if !strings.Contains(out, want) {
			t.Fatalf("assembled output missing %q\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"__PAGEZERO", "\x00"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("assembled output contains %q\n%s", unwanted, out)
		}
	}
}

func TestAssembleUsesFenceLongerThanNestedBackticks(t *testing.T) {
	dir := t.TempDir()
	rawDir := filepath.Join(dir, "raw")
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "before\n```\ncode\n```\nafter"
	if err := os.WriteFile(filepath.Join(rawDir, "claude-aesthetic.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "claude-aesthetic.err"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := synth.Assemble(synth.Input{
		RunDir:    dir,
		Reviewers: []string{"claude-aesthetic"},
		Paths: map[string]synth.Paths{
			"claude-aesthetic": {
				Raw: filepath.Join(rawDir, "claude-aesthetic.md"),
				Log: filepath.Join(logDir, "claude-aesthetic.err"),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "````markdown") {
		t.Fatalf("expected outer fence to exceed nested backticks:\n%s", out)
	}
}

func TestAssembleRereviewIncludesContext(t *testing.T) {
	dir := t.TempDir()
	inputDir := filepath.Join(dir, "inputs")
	rawDir := filepath.Join(dir, "raw")
	logDir := filepath.Join(dir, "logs")
	for _, d := range []string{inputDir, rawDir, logDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	rereviewCtx := `{"viewer_login":"alice","prior_run":{"run_dir":"/tmp/old"}}`
	if err := os.WriteFile(filepath.Join(inputDir, "rereview-context.json"), []byte(rereviewCtx), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "context.json"), []byte(`{"pr":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "diff-stat.txt"), []byte(" 1 file changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rawDir, "claude-aesthetic.md"), []byte("new finding"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "claude-aesthetic.err"), []byte("log"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := synth.Assemble(synth.Input{
		RunDir:    dir,
		Reviewers: []string{"claude-aesthetic"},
		Rereview:  true,
		Paths: map[string]synth.Paths{
			"claude-aesthetic": {
				Raw: filepath.Join(rawDir, "claude-aesthetic.md"),
				Log: filepath.Join(logDir, "claude-aesthetic.err"),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Re-review report", "Re-review Context", "viewer_login", "Diff Stat", "new finding"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestAssembleNormalOmitsRereviewPrompt(t *testing.T) {
	dir := t.TempDir()
	inputDir := filepath.Join(dir, "inputs")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "rereview-context.json"), []byte(`{"viewer_login":"alice"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := synth.Assemble(synth.Input{RunDir: dir, Reviewers: nil})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "Re-review Context") {
		t.Fatalf("normal assembly should not include re-review context:\n%s", out)
	}
}
