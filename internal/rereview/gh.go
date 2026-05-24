package rereview

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// GHClient uses the gh CLI for GitHub API access.
type GHClient struct{}

func NewGHClient() *GHClient {
	return &GHClient{}
}

func (c *GHClient) ViewerLogin() (string, error) {
	out, err := exec.Command("gh", "api", "user", "--jq", ".login").Output()
	if err != nil {
		if _, lookErr := exec.LookPath("gh"); lookErr != nil {
			return "", fmt.Errorf("gh not found in PATH")
		}
		return "", fmt.Errorf("gh api user: %w", err)
	}
	login := string(out)
	if login == "" {
		return "", fmt.Errorf("empty login from gh api user")
	}
	// trim newline from jq output
	for len(login) > 0 && (login[len(login)-1] == '\n' || login[len(login)-1] == '\r') {
		login = login[:len(login)-1]
	}
	if login == "" {
		return "", fmt.Errorf("empty login from gh api user")
	}
	return login, nil
}

func (c *GHClient) PRView(number int) (*GHPRView, error) {
	out, err := exec.Command(
		"gh", "pr", "view", fmt.Sprint(number),
		"--json", "number,title,url,author,baseRefName,headRefName,headRefOid,comments,reviews",
	).Output()
	if err != nil {
		return nil, err
	}
	var pr GHPRView
	if err := json.Unmarshal(out, &pr); err != nil {
		return nil, err
	}

	threads, err := c.fetchReviewThreads(number)
	if err != nil {
		return nil, fmt.Errorf("fetch review threads: %w", err)
	}
	pr.ReviewThreads = threads
	return &pr, nil
}

func (c *GHClient) fetchReviewThreads(number int) ([]ghReviewThread, error) {
	repoOut, err := exec.Command("gh", "repo", "view", "--json", "owner,name").Output()
	if err != nil {
		return nil, fmt.Errorf("gh repo view: %w", err)
	}
	var repo struct {
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(repoOut, &repo); err != nil {
		return nil, fmt.Errorf("parse repo json: %w", err)
	}

	const query = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: 100) {
        nodes {
          id
          isResolved
          comments(first: 50) {
            nodes {
              id
              body
              path
              line
              originalLine
              createdAt
              url
              author { login }
            }
          }
        }
      }
    }
  }
}`

	out, err := exec.Command(
		"gh", "api", "graphql",
		"-f", "query="+query,
		"-f", "owner="+repo.Owner.Login,
		"-f", "name="+repo.Name,
		"-F", fmt.Sprintf("number=%d", number),
	).Output()
	if err != nil {
		return nil, err
	}

	var payload struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						Nodes []struct {
							ID         string `json:"id"`
							IsResolved bool   `json:"isResolved"`
							Comments   struct {
								Nodes []ghThreadComment `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, fmt.Errorf("parse graphql response: %w", err)
	}

	nodes := payload.Data.Repository.PullRequest.ReviewThreads.Nodes
	threads := make([]ghReviewThread, 0, len(nodes))
	for _, node := range nodes {
		threads = append(threads, ghReviewThread{
			ID:         node.ID,
			IsResolved: node.IsResolved,
			Comments:   node.Comments.Nodes,
		})
	}
	return threads, nil
}

// FixtureClient returns canned GitHub data for tests.
type FixtureClient struct {
	Viewer string
	PR     *GHPRView
	Err    error
}

func (c *FixtureClient) ViewerLogin() (string, error) {
	if c.Err != nil {
		return "", c.Err
	}
	return c.Viewer, nil
}

func (c *FixtureClient) PRView(int) (*GHPRView, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	return c.PR, nil
}

// FakeClient returns stub data for fake reviewer integration runs.
func FakeClient(pr int) *FixtureClient {
	return &FixtureClient{
		Viewer: "reviewstack-tester",
		PR: &GHPRView{
			Number:      pr,
			Title:       fmt.Sprintf("PR %d", pr),
			URL:         fmt.Sprintf("https://github.com/example/repo/pull/%d", pr),
			BaseRefName: "main",
			HeadRefName: "feature",
			Author:      ghUser{Login: "other-dev"},
			Comments: []ghIssueComment{
				{
					ID:     1,
					Body:   "Please add tests",
					Author: ghUser{Login: "reviewstack-tester"},
				},
			},
		},
	}
}
