package launcher

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/wescopeland/reviewstack/internal/cli"
)

// Result is applied to cli.Options after the launcher exits.
type Result struct {
	Cancelled   bool
	Mode        cli.ReviewMode
	PR          int
	Base        string
	Target      string
	Uncommitted bool
	Rereview    bool
}

type PRItem struct {
	Number  int
	Title   string
	HeadRef string
	BaseRef string
}

func FetchPRs(limit int) ([]PRItem, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("gh not found — install GitHub CLI or enter a PR number manually")
	}
	out, err := exec.Command(
		"gh", "pr", "list",
		"--state", "open",
		"--limit", fmt.Sprint(limit),
		"--json", "number,title,headRefName,baseRefName",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	var rows []struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		HeadRefName string `json:"headRefName"`
		BaseRefName string `json:"baseRefName"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, err
	}
	items := make([]PRItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, PRItem{
			Number:  r.Number,
			Title:   r.Title,
			HeadRef: r.HeadRefName,
			BaseRef: r.BaseRefName,
		})
	}
	return items, nil
}

func ApplyResult(opts *cli.Options, r Result) {
	if r.Cancelled {
		return
	}
	opts.Mode = r.Mode
	opts.PR = r.PR
	opts.Base = r.Base
	opts.Target = r.Target
	opts.Uncommitted = r.Uncommitted
	opts.Rereview = r.Rereview
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
