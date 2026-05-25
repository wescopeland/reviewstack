package run_test

import (
	"testing"

	"github.com/wescopeland/reviewstack/internal/run"
)

func TestExpandArgs(t *testing.T) {
	ctx := run.NewContext("/repo", "/repo/.reviewstack/runs/x", "upstream/master", "HEAD", 4914)
	args := ctx.ExpandArgs([]string{
		"review",
		"--base",
		"{{base}}",
		"{{inputs}}/context.json",
	})
	if args[2] != "upstream/master" {
		t.Fatalf("base = %q", args[2])
	}
	if args[3] != "/repo/.reviewstack/runs/x/inputs/context.json" {
		t.Fatalf("inputs = %q", args[3])
	}
}

func TestExpandTargetDescriptionPR(t *testing.T) {
	ctx := run.NewContext("/repo", "/repo/run", "upstream/master", "HEAD", 4914)
	if got := ctx.Expand("Review {{target_description}}"); got != "Review PR 4914 against upstream/master" {
		t.Fatalf("got %q", got)
	}
}

func TestExpandTargetDescriptionUncommitted(t *testing.T) {
	ctx := run.NewContext("/repo", "/repo/run", "HEAD", "working tree", 0)
	ctx.Mode = "uncommitted"
	if got := ctx.Expand("Review {{target_description}}"); got != "Review uncommitted local changes" {
		t.Fatalf("got %q", got)
	}
}

func TestExpandReviewTargetFlags(t *testing.T) {
	prCtx := run.NewContext("/repo", "/repo/run", "main", "HEAD", 1)
	if got := prCtx.ExpandArgs([]string{"review", "{{review_target_flags}}", "prompt"}); len(got) != 2 || got[1] != "prompt" {
		t.Fatalf("PR mode args = %v", got)
	}

	uncommitted := run.NewContext("/repo", "/repo/run", "HEAD", "working tree", 0)
	uncommitted.Mode = "uncommitted"
	got := uncommitted.ExpandArgs([]string{"review", "{{review_target_flags}}", "prompt"})
	if len(got) != 2 || got[1] != "prompt" {
		t.Fatalf("uncommitted args = %v", got)
	}
}

func TestExpandPREmpty(t *testing.T) {
	ctx := run.NewContext("/repo", "/repo/run", "main", "HEAD", 0)
	if got := ctx.Expand("pr={{pr}}"); got != "pr=" {
		t.Fatalf("got %q", got)
	}
}
