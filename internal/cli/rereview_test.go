package cli_test

import (
	"strings"
	"testing"

	"github.com/wescopeland/reviewstack/internal/cli"
)

func TestParseRereview(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--rereview", "--pr", "4914"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Rereview || opts.PR != 4914 {
		t.Fatalf("opts = %+v", opts)
	}
	if err := opts.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRereviewRejectsUncommitted(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--rereview", "--uncommitted"})
	if err != nil {
		t.Fatal(err)
	}
	err = opts.Validate()
	if err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("err = %v", err)
	}
}

func TestRereviewRequiresPR(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--rereview"})
	if err != nil {
		t.Fatal(err)
	}
	err = opts.Validate()
	if err == nil || !strings.Contains(err.Error(), "requires --pr") {
		t.Fatalf("err = %v", err)
	}
}

func TestNormalModesUnchangedWithRereviewFlagOff(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--pr", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Rereview {
		t.Fatal("expected rereview false")
	}
}
