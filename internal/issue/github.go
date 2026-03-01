package issue

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"
)

// GitHub fetches issues from GitHub.
type GitHub struct {
	client      *github.Client
	defaultRepo string // "owner/repo"
}

// NewGitHub creates a GitHub issue source.
// token may be empty for public repos (but rate-limited).
// defaultRepo is used when the ref is just a number (e.g. "42").
func NewGitHub(token, defaultRepo string) *GitHub {
	var client *github.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		tc := oauth2.NewClient(context.Background(), ts)
		client = github.NewClient(tc)
	} else {
		client = github.NewClient(nil)
	}
	return &GitHub{client: client, defaultRepo: defaultRepo}
}

// Fetch fetches a GitHub issue.
// ref may be:
//   - "42"              → defaultRepo#42
//   - "owner/repo#42"  → explicit repo
func (g *GitHub) Fetch(ctx context.Context, ref string) (*Issue, error) {
	owner, repo, number, err := parseGitHubRef(ref, g.defaultRepo)
	if err != nil {
		return nil, err
	}

	gh, _, err := g.client.Issues.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("github: get issue %s/%s#%d: %w", owner, repo, number, err)
	}

	body := ""
	if gh.Body != nil {
		body = *gh.Body
	}

	return &Issue{
		ID:    fmt.Sprintf("github:%s/%s#%d", owner, repo, number),
		Title: gh.GetTitle(),
		Body:  body,
		URL:   gh.GetHTMLURL(),
	}, nil
}

func parseGitHubRef(ref, defaultRepo string) (owner, repo string, number int, err error) {
	// "owner/repo#42"
	if idx := strings.Index(ref, "#"); idx != -1 {
		parts := strings.SplitN(ref[:idx], "/", 2)
		if len(parts) != 2 {
			return "", "", 0, fmt.Errorf("invalid github ref %q: expected owner/repo#number", ref)
		}
		owner, repo = parts[0], parts[1]
		n, e := strconv.Atoi(ref[idx+1:])
		if e != nil {
			return "", "", 0, fmt.Errorf("invalid issue number in ref %q: %w", ref, e)
		}
		return owner, repo, n, nil
	}

	// Plain number → use default repo.
	n, e := strconv.Atoi(ref)
	if e != nil {
		return "", "", 0, fmt.Errorf("github ref %q: expected owner/repo#number or a plain number", ref)
	}
	parts := strings.SplitN(defaultRepo, "/", 2)
	if len(parts) != 2 {
		return "", "", 0, fmt.Errorf("no default github repo configured; use owner/repo#number format")
	}
	return parts[0], parts[1], n, nil
}
