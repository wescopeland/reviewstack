package config_test

import (
	"strings"
	"testing"

	"github.com/wescopeland/reviewstack/internal/config"
)

func TestParseValidConfig(t *testing.T) {
	data := []byte(`
reviewers:
  - id: claude-aesthetic
    command: claude
    args: ["--print", "/aesthetic-review"]
  - id: codex-medium
    command: codex
    args: ["review"]
synthesis:
  command: codex
  args: ["exec"]
`)
	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(cfg.Reviewers) != 2 {
		t.Fatalf("got %d reviewers, want 2", len(cfg.Reviewers))
	}
	if cfg.Synthesis.Command != "codex" {
		t.Fatalf("synthesis command = %q", cfg.Synthesis.Command)
	}
}

func TestParseRejectsDuplicateIDs(t *testing.T) {
	data := []byte(`
reviewers:
  - id: claude-aesthetic
    command: claude
  - id: claude-aesthetic
    command: claude
synthesis:
  command: codex
`)
	_, err := config.Parse(data)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestFilterReviewers(t *testing.T) {
	cfg := config.Default()
	filtered, err := cfg.FilterReviewers([]string{"claude-aesthetic", "codex-medium"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 {
		t.Fatalf("got %d reviewers, want 2", len(filtered))
	}
	if filtered[0].ID != "claude-aesthetic" || filtered[1].ID != "codex-medium" {
		t.Fatalf("unexpected order/ids: %+v", filtered)
	}
}

func TestFilterReviewersUnknownID(t *testing.T) {
	cfg := config.Default()
	_, err := cfg.FilterReviewers([]string{"missing"})
	if err == nil {
		t.Fatal("expected error for unknown reviewer id")
	}
}
