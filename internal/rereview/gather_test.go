package rereview_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wescopeland/reviewstack/internal/memory"
	"github.com/wescopeland/reviewstack/internal/rereview"
)

func TestGatherFiltersViewerComments(t *testing.T) {
	dir := t.TempDir()
	client := &rereview.FixtureClient{
		Viewer: "alice",
		PR: rereview.GHPRViewForTest(
			10,
			"bob",
			[]rereview.TestIssueComment{
				{ID: 1, Body: "mine", Author: "alice"},
				{ID: 2, Body: "theirs", Author: "bob"},
			},
			nil,
			nil,
		),
	}
	if err := rereview.Gather(dir, rereview.Options{PR: 10}, client); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "rereview-context.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ctx rereview.Context
	if err := json.Unmarshal(data, &ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.ViewerLogin != "alice" {
		t.Fatalf("viewer = %q", ctx.ViewerLogin)
	}
	if len(ctx.IssueComments) != 1 || ctx.IssueComments[0].Body != "mine" {
		t.Fatalf("issue comments = %+v", ctx.IssueComments)
	}
	if ctx.NoPriorRun != true {
		t.Fatal("expected no_prior_run")
	}
}

func TestGatherRejectsOwnPR(t *testing.T) {
	dir := t.TempDir()
	client := &rereview.FixtureClient{
		Viewer: "alice",
		PR:     rereview.GHPRViewForTest(10, "alice", nil, nil, nil),
	}
	err := rereview.Gather(dir, rereview.Options{PR: 10}, client)
	if err == nil || !strings.Contains(err.Error(), "someone else's PR") {
		t.Fatalf("err = %v", err)
	}
}

func TestGatherIncludesPriorRun(t *testing.T) {
	dir := t.TempDir()
	priorDir := filepath.Join(dir, "prior-run")
	if err := os.MkdirAll(priorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	final := "# Prior synthesis\n- old finding"
	if err := os.WriteFile(filepath.Join(priorDir, "final-review.md"), []byte(final), 0o644); err != nil {
		t.Fatal(err)
	}
	prior := &memory.RunRecord{
		RunDir:         priorDir,
		Timestamp:      time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		PR:             10,
		HasFinalReview: true,
		Title:          "Old title",
	}
	client := &rereview.FixtureClient{
		Viewer: "alice",
		PR:     rereview.GHPRViewForTest(10, "bob", nil, nil, nil),
	}
	if err := rereview.Gather(dir, rereview.Options{PR: 10, PriorRun: prior}, client); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "rereview-context.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ctx rereview.Context
	if err := json.Unmarshal(data, &ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.PriorRun == nil || !strings.Contains(ctx.PriorRun.FinalReviewExcerpt, "old finding") {
		t.Fatalf("prior run = %+v", ctx.PriorRun)
	}
	if ctx.NoPriorRun {
		t.Fatal("should not set no_prior_run")
	}
}

func TestGatherStableJSON(t *testing.T) {
	dir := t.TempDir()
	client := &rereview.FixtureClient{
		Viewer: "alice",
		PR: rereview.GHPRViewForTest(
			10, "bob",
			[]rereview.TestIssueComment{{ID: 1, Body: "comment", Author: "alice"}},
			[]rereview.TestReview{{ID: 2, Body: "LGTM with notes", Author: "alice", State: "COMMENTED"}},
			[]rereview.TestThread{{
				ID: "t1", Resolved: false,
				Comments: []rereview.TestInline{{ID: 3, Body: "fix nil", Author: "alice", Path: "a.go", Line: 10}},
			}},
		),
	}
	if err := rereview.Gather(dir, rereview.Options{PR: 10}, client); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(dir, "rereview-context.json"))
	if err != nil {
		t.Fatal(err)
	}
	dir2 := t.TempDir()
	if err := rereview.Gather(dir2, rereview.Options{PR: 10}, client); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(dir2, "rereview-context.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("output not stable:\n%s\n---\n%s", first, second)
	}
}
