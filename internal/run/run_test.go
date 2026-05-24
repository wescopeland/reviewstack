package run_test

import (
	"strings"
	"testing"
	"time"

	"github.com/wescopeland/reviewstack/internal/run"
)

func TestDirNameWithPR(t *testing.T) {
	ts := time.Date(2026, 5, 24, 16, 30, 45, 0, time.UTC)
	name := run.DirName(run.NamingInput{Now: ts, PR: 4914})
	want := "20260524-163045-pr-4914"
	if name != want {
		t.Fatalf("DirName() = %q, want %q", name, want)
	}
}

func TestDirNameWithLabel(t *testing.T) {
	ts := time.Date(2026, 5, 24, 16, 30, 45, 0, time.UTC)
	name := run.DirName(run.NamingInput{Now: ts, Label: "feature/foo bar"})
	if name != "20260524-163045-feature-foo-bar" {
		t.Fatalf("DirName() = %q", name)
	}
}

func TestCreateRunLayout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ids := []string{"claude-aesthetic", "codex-medium"}
	ts := time.Date(2026, 5, 24, 16, 30, 45, 0, time.UTC)
	in := run.NamingInput{Now: ts, PR: 42}
	layout, err := run.Create(dir, in, ids)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Reviewers["claude-aesthetic"].Raw == "" {
		t.Fatal("expected raw path for claude-aesthetic")
	}
	if layout.Reviewers["codex-medium"].Log == "" {
		t.Fatal("expected log path for codex-medium")
	}

	layout2, err := run.Create(dir, in, ids)
	if err != nil {
		t.Fatal(err)
	}
	if layout2.Root == layout.Root {
		t.Fatalf("expected collision-resistant dir, both %q", layout.Root)
	}
	if !strings.HasSuffix(layout2.Root, "-1") {
		t.Fatalf("expected suffix -1 on collision, got %q", layout2.Root)
	}
}
