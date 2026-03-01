// Package pipeline implements the Sling pipeline stages.
package pipeline

import (
	"context"
	"fmt"

	"github.com/aronasorman/sling/internal/bead"
	"github.com/aronasorman/sling/internal/issue"
)

// IntakeResult is returned by Intake.
type IntakeResult struct {
	Issue  *issue.Issue
	EpicID string
}

// Intake fetches the issue identified by ref and creates an epic bead.
// Returns the epic bead ID and the fetched issue.
func Intake(ctx context.Context, ref string, src issue.Source) (*IntakeResult, error) {
	iss, err := src.Fetch(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("intake: fetch issue %s: %w", ref, err)
	}
	fmt.Printf("Fetched issue: %s — %s\n", iss.ID, iss.Title)

	body := fmt.Sprintf("Epic created from issue %s\n\nURL: %s\n\n%s", iss.ID, iss.URL, iss.Body)
	epicID, err := bead.Create(iss.Title, body, "", []string{bead.LabelPlanned})
	if err != nil {
		return nil, fmt.Errorf("intake: create epic bead: %w", err)
	}
	fmt.Printf("Created epic bead: %s\n", epicID)

	return &IntakeResult{Issue: iss, EpicID: epicID}, nil
}
