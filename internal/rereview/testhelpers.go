package rereview

// Test helpers exported to rereview_test package.

type TestIssueComment struct {
	ID     int64
	Body   string
	Author string
}

type TestReview struct {
	ID     int64
	Body   string
	Author string
	State  string
}

type TestInline struct {
	ID     int64
	Body   string
	Author string
	Path   string
	Line   int
}

type TestThread struct {
	ID       string
	Resolved bool
	Comments []TestInline
}

func GHPRViewForTest(number int, author string, issues []TestIssueComment, reviews []TestReview, threads []TestThread) *GHPRView {
	pr := &GHPRView{
		Number:      number,
		Title:       "Test PR",
		URL:         "https://github.com/o/r/pull/10",
		BaseRefName: "main",
		HeadRefName: "feat",
		Author:      ghUser{Login: author},
	}
	for _, c := range issues {
		pr.Comments = append(pr.Comments, ghIssueComment{
			ID:     c.ID,
			Body:   c.Body,
			Author: ghUser{Login: c.Author},
		})
	}
	for _, r := range reviews {
		pr.Reviews = append(pr.Reviews, ghReview{
			ID:     r.ID,
			Body:   r.Body,
			State:  r.State,
			Author: ghUser{Login: r.Author},
		})
	}
	for _, th := range threads {
		thread := ghReviewThread{ID: th.ID, IsResolved: th.Resolved}
		for _, c := range th.Comments {
			thread.Comments = append(thread.Comments, ghThreadComment{
				ID:     c.ID,
				Body:   c.Body,
				Path:   c.Path,
				Line:   c.Line,
				Author: ghUser{Login: c.Author},
			})
		}
		pr.ReviewThreads = append(pr.ReviewThreads, thread)
	}
	return pr
}
