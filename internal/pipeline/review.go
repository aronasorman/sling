package pipeline

import (
	"fmt"

	"github.com/aronasorman/sling/internal/agent"
	"github.com/aronasorman/sling/internal/bead"
	"github.com/aronasorman/sling/internal/notify"
	"github.com/aronasorman/sling/internal/worktree"
)

// AutoReviewOptions configures the automated review pipeline.
type AutoReviewOptions struct {
	MaxRounds    int
	Notifier     *notify.Notifier
	ContextFiles map[string]string
}

// RunAutomatedReview runs the Reviewer → Addresser loop up to MaxRounds times.
// When the worktree is clean (no REVIEW: markers), it squashes the review
// commits into the work commit. It always labels the bead sling:review-pending
// and sends a notification when finished.
func RunAutomatedReview(beadID, wtPath string, opts AutoReviewOptions) error {
	if opts.MaxRounds < 1 {
		opts.MaxRounds = 3
	}
	if opts.Notifier == nil {
		opts.Notifier = notify.New(false, "", "")
	}

	b, err := bead.Show(beadID)
	if err != nil {
		return fmt.Errorf("automated-review: fetch bead: %w", err)
	}

	clean := false
	for round := 1; round <= opts.MaxRounds; round++ {
		fmt.Printf("Automated review round %d/%d for bead %s\n", round, opts.MaxRounds, beadID)

		// Reviewer agent: adversarially reviews the diff and adds REVIEW: markers.
		reviewerPrompt := agent.ReviewerSystemPrompt(b.Title, opts.ContextFiles)
		reviewerUser := fmt.Sprintf(
			"Review the implementation of bead: %s\n\nBead description:\n%s\n\n"+
				"Run `jj diff` first to see the changes, then add REVIEW: markers for any issues you find.",
			b.Title, b.Body,
		)
		if err := agent.Run(agent.RunOptions{
			WorkDir:      wtPath,
			SystemPrompt: reviewerPrompt,
			UserPrompt:   reviewerUser,
			Model:        agent.ModelSonnet,
			MaxTurns:     30,
		}); err != nil {
			fmt.Printf("Warning: reviewer agent error on round %d: %v\n", round, err)
		}

		hasMarkers, err := HasReviewMarkers(wtPath)
		if err != nil {
			return fmt.Errorf("automated-review: check markers (round %d): %w", round, err)
		}
		if !hasMarkers {
			fmt.Printf("No REVIEW: markers found after round %d review — automated review clean.\n", round)
			clean = true
			break
		}

		fmt.Printf("REVIEW: markers found on round %d, running Addresser...\n", round)

		// Addresser agent: resolves REVIEW: markers and runs tests.
		addresserPrompt := agent.AddresserSystemPrompt(b.Title, opts.ContextFiles)
		addresserUser := fmt.Sprintf(
			"Address all REVIEW: markers in bead: %s\n\nBead description:\n%s\n\n"+
				"Find all REVIEW: markers with grep, fix each issue, remove the marker, run tests.",
			b.Title, b.Body,
		)
		if err := agent.Run(agent.RunOptions{
			WorkDir:      wtPath,
			SystemPrompt: addresserPrompt,
			UserPrompt:   addresserUser,
			Model:        agent.ModelSonnet,
			MaxTurns:     40,
		}); err != nil {
			fmt.Printf("Warning: addresser agent error on round %d: %v\n", round, err)
		}

		// Re-check after addressing.
		hasMarkers, err = HasReviewMarkers(wtPath)
		if err != nil {
			return fmt.Errorf("automated-review: re-check markers (round %d): %w", round, err)
		}
		if !hasMarkers {
			clean = true
			fmt.Printf("REVIEW: markers resolved after addressing on round %d.\n", round)
			break
		}
	}

	if clean {
		// Squash all review/address commits back into the work commit.
		squashMsg := fmt.Sprintf("feat(%s): %s", beadID, b.Title)
		if err := worktree.Squash(wtPath, squashMsg); err != nil {
			fmt.Printf("Warning: could not squash review commits: %v\n", err)
		}
		fmt.Printf("Automated review complete and clean for bead %s.\n", beadID)
	} else {
		fmt.Printf("REVIEW: markers remain after %d rounds for bead %s — human review needed.\n", opts.MaxRounds, beadID)
	}

	// Always transition to review-pending and notify.
	if err := bead.SwapSlingLabel(beadID, bead.LabelReviewPending); err != nil {
		fmt.Printf("Warning: could not label bead as review-pending: %v\n", err)
	}
	_ = opts.Notifier.Send(fmt.Sprintf("Sling: bead %q is ready for review. ID: %s", b.Title, beadID))

	return nil
}
