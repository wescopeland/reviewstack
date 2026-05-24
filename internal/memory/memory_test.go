package memory_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wescopeland/reviewstack/internal/memory"
	"github.com/wescopeland/reviewstack/internal/run"
)

func TestIndexAndLoadPR(t *testing.T) {
	base := t.TempDir()
	ts := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	rec := memory.RunRecord{
		RunDir:         filepath.Join(base, "run-a"),
		Timestamp:      ts,
		PR:             42,
		Title:          "Fix bug",
		HasFinalReview: true,
	}
	if err := memory.IndexRun(base, rec); err != nil {
		t.Fatal(err)
	}

	runs, err := memory.LoadPR(base, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d", len(runs))
	}
	if runs[0].Title != "Fix bug" {
		t.Fatalf("title = %q", runs[0].Title)
	}
}

func TestSelectPriorRunPrefersFinalReview(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, "old")
	newDir := filepath.Join(base, "new")
	for _, d := range []string{oldDir, newDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(oldDir, run.FinalReview), []byte("# old"), 0o644); err != nil {
		t.Fatal(err)
	}

	ts := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	runs := []memory.RunRecord{
		{RunDir: oldDir, Timestamp: ts, PR: 1, HasFinalReview: true},
		{RunDir: newDir, Timestamp: ts.Add(time.Hour), PR: 1, HasFinalReview: false},
	}
	prior := memory.SelectPriorRun(runs, newDir)
	if prior == nil {
		t.Fatal("expected prior run")
	}
	if prior.RunDir != oldDir {
		t.Fatalf("prior = %q want %q", prior.RunDir, oldDir)
	}
}

func TestSelectPriorRunIgnoresMissingDir(t *testing.T) {
	base := t.TempDir()
	existing := filepath.Join(base, "exists")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	runs := []memory.RunRecord{
		{RunDir: filepath.Join(base, "gone"), Timestamp: time.Now(), PR: 1},
		{RunDir: existing, Timestamp: time.Now(), PR: 1},
	}
	prior := memory.SelectPriorRun(runs, filepath.Join(base, "current"))
	if prior == nil || prior.RunDir != existing {
		t.Fatalf("prior = %v", prior)
	}
}

func TestRecordFromRunDir(t *testing.T) {
	base := t.TempDir()
	runDir := filepath.Join(base, "run")
	inputs := filepath.Join(runDir, run.InputsDir)
	if err := os.MkdirAll(inputs, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := `{"title":"My PR","url":"https://github.com/o/r/pull/7","base":"main","target":"feat"}`
	if err := os.WriteFile(filepath.Join(inputs, "context.json"), []byte(ctx), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, run.FinalReview), []byte("# done"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := memory.RecordFromRunDir(runDir, 7, false)
	if rec.Title != "My PR" || !rec.HasFinalReview || rec.Head != "feat" {
		t.Fatalf("rec = %+v", rec)
	}
}
