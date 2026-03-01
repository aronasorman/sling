// Package issue provides the IssueSource interface and implementations for
// fetching issues from GitHub and Linear.
package issue

import (
	"context"
	"fmt"
	"strings"
)

// Issue is the canonical representation of an issue from any source.
type Issue struct {
	ID    string // e.g. "github:aronasorman/sling#42" or "LIN-423"
	Title string
	Body  string
	URL   string
}

// Source can fetch a single issue by its reference string.
type Source interface {
	Fetch(ctx context.Context, ref string) (*Issue, error)
}

// DetectSource returns a Source based on the ref string and configured source.
// ref examples: "LIN-423", "42", "owner/repo#42"
// When configured is "" or "description", a DescriptionSource is returned as a
// fallback so that callers without a remote issue tracker can still proceed.
func DetectSource(configured, ref, githubToken, linearToken, defaultRepo string) (Source, error) {
	switch configured {
	case "github":
		return NewGitHub(githubToken, defaultRepo), nil
	case "linear":
		return NewLinear(linearToken), nil
	case "auto":
		if looksLikeLinear(ref) {
			return NewLinear(linearToken), nil
		}
		return NewGitHub(githubToken, defaultRepo), nil
	case "", "description":
		// No remote tracker configured – fall back to a local DescriptionSource.
		// ref is used as the Issue Title so the epic bead gets a meaningful name.
		return NewDescriptionSource(ref, ""), nil
	default:
		return nil, fmt.Errorf("unknown issue_source %q; use github, linear, auto, or description", configured)
	}
}

func looksLikeLinear(ref string) bool {
	// Linear issue IDs look like "ENG-123" or "LIN-42".
	parts := strings.SplitN(ref, "-", 2)
	if len(parts) != 2 {
		return false
	}
	if len(parts[0]) == 0 || len(parts[1]) == 0 {
		return false
	}
	// prefix must be all uppercase letters
	for _, c := range parts[0] {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	// suffix must be all digits
	for _, c := range parts[1] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
