package launcher_test

import (
	"testing"

	"github.com/wescopeland/reviewstack/internal/cli"
	"github.com/wescopeland/reviewstack/internal/launcher"
)

func TestApplyResultRereview(t *testing.T) {
	opts := &cli.Options{}
	result := launcher.Result{
		Mode:     cli.ModePR,
		PR:       42,
		Base:     "main",
		Target:   "feat",
		Rereview: true,
	}
	launcher.ApplyResult(opts, result)
	if !opts.Rereview || opts.PR != 42 || opts.Mode != cli.ModePR {
		t.Fatalf("opts = %+v", opts)
	}
	if err := opts.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyResultNormalPRUnchanged(t *testing.T) {
	opts := &cli.Options{}
	result := launcher.Result{Mode: cli.ModePR, PR: 7, Base: "main", Target: "HEAD"}
	launcher.ApplyResult(opts, result)
	if opts.Rereview {
		t.Fatal("expected rereview false")
	}
}
