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

// IntakeOpts holds optional parameters for Intake.
type IntakeOpts struct {
	// Rig is the gastown rig name to create beads in (e.g. "sling").
	// When empty, beads are created using default bd routing.
	Rig string
}

// Intake fetches the issue identified by ref and creates an epic bead.
// Returns the epic bead ID and the fetched issue.
func Intake(ctx context.Context, ref string, src issue.Source, opts ...IntakeOpts) (*IntakeResult, error) {
	opt := IntakeOpts{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	iss, err := src.Fetch(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("intake: fetch issue %s: %w", ref, err)
	}
	fmt.Printf("Fetched issue: %s — %s\n", iss.ID, iss.Title)

	var body string
	if iss.URL != "" {
		body = fmt.Sprintf("Epic created from issue %s\n\nURL: %s\n\n%s", iss.ID, iss.URL, iss.Body)
	} else {
		body = fmt.Sprintf("Epic created from: %s\n\n%s", iss.ID, iss.Body)
	}
	var epicID string
	if opt.Rig != "" {
		epicID, err = bead.CreateInRig(opt.Rig, iss.Title, body, "", []string{bead.LabelPlanned})
	} else {
		epicID, err = bead.Create(iss.Title, body, "", []string{bead.LabelPlanned})
	}
	if err != nil {
		return nil, fmt.Errorf("intake: create epic bead: %w", err)
	}
	fmt.Printf("Created epic bead: %s\n", epicID)

	return &IntakeResult{Issue: iss, EpicID: epicID}, nil
}
