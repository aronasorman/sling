package issue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const linearAPIURL = "https://api.linear.app/graphql"

// Linear fetches issues from Linear via the GraphQL API.
type Linear struct {
	apiKey string
}

// NewLinear creates a Linear issue source.
func NewLinear(apiKey string) *Linear {
	return &Linear{apiKey: apiKey}
}

// Fetch fetches a Linear issue. ref should be the issue identifier (e.g. "ENG-123").
func (l *Linear) Fetch(ctx context.Context, ref string) (*Issue, error) {
	query := `
query($id: String!) {
  issue(id: $id) {
    identifier
    title
    description
    url
  }
}`

	payload := map[string]any{
		"query":     query,
		"variables": map[string]string{"id": ref},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, linearAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", l.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linear: request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("linear: read response: %w", err)
	}

	var result struct {
		Data struct {
			Issue struct {
				Identifier  string `json:"identifier"`
				Title       string `json:"title"`
				Description string `json:"description"`
				URL         string `json:"url"`
			} `json:"issue"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("linear: decode response: %w", err)
	}
	if len(result.Errors) > 0 {
		msgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("linear API errors: %s", strings.Join(msgs, "; "))
	}

	iss := result.Data.Issue
	return &Issue{
		ID:    iss.Identifier,
		Title: iss.Title,
		Body:  iss.Description,
		URL:   iss.URL,
	}, nil
}
