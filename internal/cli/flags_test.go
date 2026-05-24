package cli_test

import (
	"strings"
	"testing"

	"github.com/wescopeland/reviewstack/internal/cli"
)

func TestParsePRNumber(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--pr", "4914"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.PR != 4914 {
		t.Fatalf("PR = %d", opts.PR)
	}
}

func TestParsePRURL(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--pr", "https://github.com/org/repo/pull/4914"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.PR != 4914 {
		t.Fatalf("PR = %d", opts.PR)
	}
}

func TestParsePRURLWithFilesSuffix(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--pr", "https://github.com/org/repo/pull/4914/files"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.PR != 4914 {
		t.Fatalf("PR = %d", opts.PR)
	}
}

func TestParsePRURLWithQuery(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--pr", "https://github.com/org/repo/pull/4914?tab=files"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.PR != 4914 {
		t.Fatalf("PR = %d", opts.PR)
	}
}

func TestUncommittedRejectsPR(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--uncommitted", "--pr", "4914"})
	if err != nil {
		t.Fatal(err)
	}
	err = opts.Validate()
	if err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("err = %v", err)
	}
}

func TestSynthesizeOnlyWithoutPR(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--synthesize-only", ".reviewstack/runs/example"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.SynthesizeOnly == "" {
		t.Fatal("expected synthesize-only to be set")
	}
}

func TestOnlyCSV(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--pr", "1", "--only", "claude-aesthetic,codex-medium"})
	if err != nil {
		t.Fatal(err)
	}
	if len(opts.Only) != 2 {
		t.Fatalf("Only = %v", opts.Only)
	}
}
