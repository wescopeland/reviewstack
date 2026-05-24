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

func TestExpandPREmpty(t *testing.T) {
	ctx := run.NewContext("/repo", "/repo/run", "main", "HEAD", 0)
	if got := ctx.Expand("pr={{pr}}"); got != "pr=current" {
		t.Fatalf("got %q", got)
	}
}
