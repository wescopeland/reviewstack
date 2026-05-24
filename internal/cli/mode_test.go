package cli_test

import (
	"testing"

	"github.com/wescopeland/reviewstack/internal/cli"
)

func TestNeedsLauncherDefault(t *testing.T) {
	opts, err := cli.ParseArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.NeedsLauncher() {
		t.Fatal("expected launcher with no args")
	}
}

func TestNeedsLauncherPR(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--pr", "4914"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.NeedsLauncher() {
		t.Fatal("should not launch picker when --pr set")
	}
	if opts.Mode != cli.ModePR {
		t.Fatalf("mode = %v", opts.Mode)
	}
}

func TestNeedsLauncherUncommitted(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--uncommitted"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.NeedsLauncher() {
		t.Fatal("should not launch picker when --uncommitted set")
	}
}

func TestNeedsLauncherNoLauncherFlag(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--no-launcher"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.NeedsLauncher() {
		t.Fatal("expected --no-launcher to disable picker")
	}
	if opts.HasReviewTarget() {
		t.Fatal("expected no review target without flags")
	}
}
