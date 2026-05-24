package rereview

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wescopeland/reviewstack/internal/memory"
)

// Context is written to inputs/rereview-context.json.
type Context struct {
	ViewerLogin     string          `json:"viewer_login"`
	PR              PRInfo          `json:"pr"`
	InlineComments  []InlineComment `json:"inline_comments"`
	IssueComments   []IssueComment  `json:"issue_comments"`
	ReviewSummaries []ReviewSummary `json:"review_summaries"`
	PriorRun        *PriorRun       `json:"prior_run,omitempty"`
	NoPriorRun      bool            `json:"no_prior_run,omitempty"`
	NoPriorRunNote  string          `json:"no_prior_run_note,omitempty"`
}

type PRInfo struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Author  string `json:"author"`
	BaseRef string `json:"base_ref"`
	HeadRef string `json:"head_ref"`
	HeadSHA string `json:"head_sha,omitempty"`
}

type InlineComment struct {
	ID           int64  `json:"id"`
	Body         string `json:"body"`
	Path         string `json:"path,omitempty"`
	Line         int    `json:"line,omitempty"`
	OriginalLine int    `json:"original_line,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
	URL          string `json:"url,omitempty"`
	ThreadID     string `json:"thread_id,omitempty"`
	Resolved     *bool  `json:"resolved,omitempty"`
}

type IssueComment struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at,omitempty"`
	URL       string `json:"url,omitempty"`
}

type ReviewSummary struct {
	ID        int64  `json:"id"`
	State     string `json:"state"`
	Body      string `json:"body"`
	Submitted string `json:"submitted_at,omitempty"`
	URL       string `json:"url,omitempty"`
}

type PriorRun struct {
	RunDir             string `json:"run_dir"`
	Timestamp          string `json:"timestamp"`
	HasFinalReview     bool   `json:"has_final_review"`
	FinalReviewPath    string `json:"final_review_path,omitempty"`
	FinalReviewExcerpt string `json:"final_review_excerpt,omitempty"`
	Title              string `json:"title,omitempty"`
	URL                string `json:"url,omitempty"`
	Base               string `json:"base,omitempty"`
	Head               string `json:"head,omitempty"`
	HeadSHA            string `json:"head_sha,omitempty"`
}

type Options struct {
	PR         int
	CurrentRun string
	RepoRoot   string
	PriorRun   *memory.RunRecord
}

// Client fetches GitHub data for re-review context gathering.
type Client interface {
	ViewerLogin() (string, error)
	PRView(number int) (*GHPRView, error)
}

type GHPRView struct {
	Number        int              `json:"number"`
	Title         string           `json:"title"`
	URL           string           `json:"url"`
	BaseRefName   string           `json:"baseRefName"`
	HeadRefName   string           `json:"headRefName"`
	HeadRefOid    string           `json:"headRefOid"`
	Author        ghUser           `json:"author"`
	Comments      []ghIssueComment `json:"comments"`
	Reviews       []ghReview       `json:"reviews"`
	ReviewThreads []ghReviewThread `json:"reviewThreads"`
}

type ghUser struct {
	Login string `json:"login"`
}

type ghIssueComment struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	URL       string `json:"url"`
	Author    ghUser `json:"author"`
}

type ghReview struct {
	ID          int64  `json:"id"`
	State       string `json:"state"`
	Body        string `json:"body"`
	SubmittedAt string `json:"submittedAt"`
	URL         string `json:"url"`
	Author      ghUser `json:"author"`
}

type ghReviewThread struct {
	ID         string            `json:"id"`
	IsResolved bool              `json:"isResolved"`
	Comments   []ghThreadComment `json:"comments"`
}

type ghThreadComment struct {
	ID           int64  `json:"id"`
	Body         string `json:"body"`
	Path         string `json:"path"`
	Line         int    `json:"line"`
	OriginalLine int    `json:"originalLine"`
	CreatedAt    string `json:"createdAt"`
	URL          string `json:"url"`
	Author       ghUser `json:"author"`
}

// Gather writes rereview-context.json into inputsDir using the GitHub client.
func Gather(inputsDir string, opts Options, client Client) error {
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		return err
	}

	viewer, err := client.ViewerLogin()
	if err != nil {
		return fmt.Errorf("github auth: %w (run: gh auth login)", err)
	}

	pr, err := client.PRView(opts.PR)
	if err != nil {
		return fmt.Errorf("fetch PR %d: %w", opts.PR, err)
	}

	if pr.Author.Login == viewer {
		return fmt.Errorf("re-review is for someone else's PR; you authored PR #%d — use a normal review instead", opts.PR)
	}

	ctx := Context{
		ViewerLogin: viewer,
		PR: PRInfo{
			Number:  pr.Number,
			Title:   pr.Title,
			URL:     pr.URL,
			Author:  pr.Author.Login,
			BaseRef: pr.BaseRefName,
			HeadRef: pr.HeadRefName,
			HeadSHA: pr.HeadRefOid,
		},
	}

	for _, c := range pr.Comments {
		if c.Author.Login != viewer {
			continue
		}
		ctx.IssueComments = append(ctx.IssueComments, IssueComment{
			ID:        c.ID,
			Body:      c.Body,
			CreatedAt: c.CreatedAt,
			URL:       c.URL,
		})
	}

	for _, r := range pr.Reviews {
		if r.Author.Login != viewer {
			continue
		}
		if strings.TrimSpace(r.Body) == "" {
			continue
		}
		ctx.ReviewSummaries = append(ctx.ReviewSummaries, ReviewSummary{
			ID:        r.ID,
			State:     r.State,
			Body:      r.Body,
			Submitted: r.SubmittedAt,
			URL:       r.URL,
		})
	}

	for _, thread := range pr.ReviewThreads {
		resolved := thread.IsResolved
		for _, c := range thread.Comments {
			if c.Author.Login != viewer {
				continue
			}
			ctx.InlineComments = append(ctx.InlineComments, InlineComment{
				ID:           c.ID,
				Body:         c.Body,
				Path:         c.Path,
				Line:         c.Line,
				OriginalLine: c.OriginalLine,
				CreatedAt:    c.CreatedAt,
				URL:          c.URL,
				ThreadID:     thread.ID,
				Resolved:     &resolved,
			})
		}
	}

	if opts.PriorRun != nil {
		prior := &PriorRun{
			RunDir:         opts.PriorRun.RunDir,
			Timestamp:      opts.PriorRun.Timestamp.UTC().Format(timeRFC3339),
			HasFinalReview: opts.PriorRun.HasFinalReview,
			Title:          opts.PriorRun.Title,
			URL:            opts.PriorRun.URL,
			Base:           opts.PriorRun.Base,
			Head:           opts.PriorRun.Head,
			HeadSHA:        opts.PriorRun.HeadSHA,
		}
		finalPath := filepath.Join(opts.PriorRun.RunDir, "final-review.md")
		if opts.PriorRun.HasFinalReview {
			prior.FinalReviewPath = finalPath
			if data, err := os.ReadFile(finalPath); err == nil {
				prior.FinalReviewExcerpt = excerpt(string(data), 4000)
			}
		}
		ctx.PriorRun = prior
	} else {
		ctx.NoPriorRun = true
		ctx.NoPriorRunNote = "No prior reviewstack run found for this PR; report uses GitHub feedback only."
	}

	return writeJSON(filepath.Join(inputsDir, "rereview-context.json"), ctx)
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

func excerpt(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n\n… (truncated)"
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
