package issue

import "context"

// DescriptionSource is a Source implementation that returns a fixed Issue
// without making any external API calls. It is useful when the issue
// description is provided directly (e.g., from a file or command-line flag)
// rather than fetched from GitHub or Linear.
type DescriptionSource struct {
	title string
	body  string
}

// NewDescriptionSource creates a DescriptionSource with the given title and body.
func NewDescriptionSource(title, body string) *DescriptionSource {
	return &DescriptionSource{title: title, body: body}
}

// Fetch returns an Issue built from the pre-defined title and body.
// The ref is used as the Issue ID. No network calls are made.
func (d *DescriptionSource) Fetch(_ context.Context, ref string) (*Issue, error) {
	return &Issue{
		ID:    ref,
		Title: d.title,
		Body:  d.body,
		URL:   "",
	}, nil
}
