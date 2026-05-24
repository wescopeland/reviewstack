package config_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wescopeland/reviewstack/internal/config"
)

func TestAdaptReviewersUncommittedCodex(t *testing.T) {
	reviewers := []config.Reviewer{{
		ID:      "codex-medium",
		Command: "codex",
		Args:    []string{"review", "read {{inputs}}/diff.patch", "-c", `model="gpt-5.5"`},
	}}
	got := config.AdaptReviewers(reviewers, true)
	want := []string{"review", "--uncommitted", "read {{inputs}}/diff.patch", "-c", `model="gpt-5.5"`}
	if !reflect.DeepEqual(got[0].Args, want) {
		t.Fatalf("args = %v, want %v", got[0].Args, want)
	}
}

func TestAdaptReviewersUncommittedCodexWithoutPromptKeepsConfigValue(t *testing.T) {
	reviewers := []config.Reviewer{{
		ID:      "codex-medium",
		Command: "codex",
		Args:    []string{"review", "--base", "main", "-c", `model="gpt-5.5"`},
	}}
	got := config.AdaptReviewers(reviewers, true)
	want := []string{"review", "--uncommitted", "-c", `model="gpt-5.5"`}
	if !reflect.DeepEqual(got[0].Args, want) {
		t.Fatalf("args = %v, want %v", got[0].Args, want)
	}
}

func TestAdaptReviewersUncommittedClaude(t *testing.T) {
	reviewers := []config.Reviewer{{
		ID:      "claude-aesthetic",
		Command: "claude",
		Args:    []string{"--print", "/aesthetic-review", "{{pr}}"},
	}}
	got := config.AdaptReviewers(reviewers, true)
	if got[0].Args[len(got[0].Args)-1] == "{{pr}}" {
		t.Fatalf("expected {{pr}} to be replaced for uncommitted mode, got %v", got[0].Args)
	}
	if !strings.Contains(got[0].Args[len(got[0].Args)-1], "{{inputs}}") {
		t.Fatalf("expected local review prompt with {{inputs}}, got %v", got[0].Args)
	}
}

func TestAdaptReviewersPRUnchanged(t *testing.T) {
	reviewers := []config.Reviewer{{
		ID:   "codex-medium",
		Args: []string{"review", "--base", "main"},
	}}
	got := config.AdaptReviewers(reviewers, false)
	if !reflect.DeepEqual(got[0].Args, reviewers[0].Args) {
		t.Fatalf("expected unchanged args")
	}
}
