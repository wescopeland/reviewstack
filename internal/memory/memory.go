package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/wescopeland/reviewstack/internal/run"
)

const MemoryDir = ".reviewstack/memory"

// RunRecord stores metadata for one indexed PR review run.
type RunRecord struct {
	RunDir         string    `json:"run_dir"`
	Timestamp      time.Time `json:"timestamp"`
	PR             int       `json:"pr"`
	Title          string    `json:"title,omitempty"`
	URL            string    `json:"url,omitempty"`
	Base           string    `json:"base,omitempty"`
	Head           string    `json:"head,omitempty"`
	HeadSHA        string    `json:"head_sha,omitempty"`
	HasFinalReview bool      `json:"has_final_review"`
	Rereview       bool      `json:"rereview,omitempty"`
}

type prMemory struct {
	PR   int         `json:"pr"`
	Runs []RunRecord `json:"runs"`
}

func prFilePath(base string, pr int) string {
	return filepath.Join(base, MemoryDir, fmt.Sprintf("pr-%d.json", pr))
}

// IndexRun appends or updates a run record for the given PR.
func IndexRun(base string, rec RunRecord) error {
	if rec.PR <= 0 {
		return fmt.Errorf("memory: PR number required")
	}
	dir := filepath.Join(base, MemoryDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	path := prFilePath(base, rec.PR)
	var mem prMemory
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &mem)
	}
	if mem.PR == 0 {
		mem.PR = rec.PR
	}

	updated := false
	for i, existing := range mem.Runs {
		if existing.RunDir == rec.RunDir {
			mem.Runs[i] = rec
			updated = true
			break
		}
	}
	if !updated {
		mem.Runs = append(mem.Runs, rec)
	}

	sort.Slice(mem.Runs, func(i, j int) bool {
		return mem.Runs[i].Timestamp.Before(mem.Runs[j].Timestamp)
	})

	data, err := json.MarshalIndent(mem, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// LoadPR returns all indexed runs for a PR.
func LoadPR(base string, pr int) ([]RunRecord, error) {
	path := prFilePath(base, pr)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var mem prMemory
	if err := json.Unmarshal(data, &mem); err != nil {
		return nil, err
	}
	return mem.Runs, nil
}

// SelectPriorRun picks the latest prior run for re-review, excluding excludeDir.
// Prefers a run with final-review.md when multiple candidates exist at the same timestamp tier.
func SelectPriorRun(runs []RunRecord, excludeDir string) *RunRecord {
	var candidates []RunRecord
	for _, r := range runs {
		if r.RunDir == excludeDir {
			continue
		}
		if _, err := os.Stat(r.RunDir); err != nil {
			continue
		}
		candidates = append(candidates, r)
	}
	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Timestamp.Before(candidates[j].Timestamp)
	})

	latest := candidates[len(candidates)-1]
	if latest.HasFinalReview {
		return &latest
	}

	// Prefer the latest run that has final-review.md, if any.
	for i := len(candidates) - 1; i >= 0; i-- {
		r := candidates[i]
		finalPath := filepath.Join(r.RunDir, run.FinalReview)
		if _, err := os.Stat(finalPath); err == nil {
			r.HasFinalReview = true
			return &r
		}
	}

	return &latest
}

// RecordFromRunDir builds a RunRecord from an on-disk run layout.
func RecordFromRunDir(runDir string, pr int, rereview bool) RunRecord {
	rec := RunRecord{
		RunDir:   runDir,
		PR:       pr,
		Rereview: rereview,
	}
	if info, err := os.Stat(runDir); err == nil {
		rec.Timestamp = info.ModTime().UTC()
	} else {
		rec.Timestamp = time.Now().UTC()
	}

	ctxPath := filepath.Join(runDir, run.InputsDir, "context.json")
	if data, err := os.ReadFile(ctxPath); err == nil {
		var ctx struct {
			Title  string `json:"title"`
			URL    string `json:"url"`
			Base   string `json:"base"`
			Target string `json:"target"`
		}
		if json.Unmarshal(data, &ctx) == nil {
			rec.Title = ctx.Title
			rec.URL = ctx.URL
			rec.Base = ctx.Base
			rec.Head = ctx.Target
		}
	}

	prPath := filepath.Join(runDir, run.InputsDir, "pr.json")
	if data, err := os.ReadFile(prPath); err == nil {
		var prData struct {
			HeadRefOid string `json:"headRefOid"`
		}
		if json.Unmarshal(data, &prData) == nil && prData.HeadRefOid != "" {
			rec.HeadSHA = prData.HeadRefOid
		}
	}

	finalPath := filepath.Join(runDir, run.FinalReview)
	if _, err := os.Stat(finalPath); err == nil {
		rec.HasFinalReview = true
	}
	return rec
}
