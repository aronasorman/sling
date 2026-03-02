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

// Injectable vars for review.go — overridden in tests.
var (
	reviewBeadShow = func(id string) (*bead.Bead, error) {
		return bead.Show(id)
	}
	reviewBeadSwapLabel = func(id, label string) error {
		return bead.SwapSlingLabel(id, label)
	}
	reviewWorktreeSquash = func(wtPath, msg string) error {
		return worktree.Squash(wtPath, msg)
	}
	reviewAgentRun = func(opts agent.RunOptions) error {
		return agent.Run(opts)
	}
)

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

// ─── Epic holistic review ─────────────────────────────────────────────────────

// EpicHolisticReviewOptions configures RunEpicHolisticReview.
type EpicHolisticReviewOptions struct {
	AutoReviewOptions              // embeds MaxRounds, Notifier, ContextFiles
	EpicTitle string               // used in reviewer prompt header
	EpicBody  string               // used in reviewer prompt for goal context
	SubBeads  []agent.EpicSubBeadSpec // provides spec context to reviewer
}

// RunEpicHolisticReview runs the Reviewer → Addresser loop for an epic worktree.
// It uses EpicReviewerSystemPrompt so the Reviewer has full cross-bead context.
// Behavior mirrors RunAutomatedReview: always labels epicID sling:review-pending
// and notifies when done.
//
// Parameters:
//
//	epicID  – the bd bead ID of the epic (used for label transitions + notifications)
//	wtPath  – absolute path to the epic's shared jj worktree
//	opts    – configuration; MaxRounds defaults to 3 if < 1
//
// Returns nil on success (even if markers remain after MaxRounds; the epic is
// still transitioned to sling:review-pending for human inspection).
func RunEpicHolisticReview(epicID, wtPath string, opts EpicHolisticReviewOptions) error {
	if opts.MaxRounds < 1 {
		opts.MaxRounds = 3
	}
	if opts.Notifier == nil {
		opts.Notifier = notify.New(false, "", "")
	}

	clean := false
	for round := 1; round <= opts.MaxRounds; round++ {
		fmt.Printf("Epic holistic review round %d/%d for epic %s\n", round, opts.MaxRounds, epicID)

		// Reviewer step — use EpicReviewerSystemPrompt for full cross-bead context.
		reviewerPrompt := agent.EpicReviewerSystemPrompt(opts.EpicTitle, opts.EpicBody, opts.SubBeads, opts.ContextFiles)
		reviewerUser := fmt.Sprintf(
			"Review the epic implementation: %s\n\nEpic description:\n%s\n\n"+
				"Run `jj diff` first to see all changes across the entire epic, then add REVIEW: markers for any issues you find.",
			opts.EpicTitle, opts.EpicBody,
		)
		if err := reviewAgentRun(agent.RunOptions{
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
			return fmt.Errorf("epic-holistic-review: check markers (round %d): %w", round, err)
		}
		if !hasMarkers {
			fmt.Printf("No REVIEW: markers found after round %d review — epic review clean.\n", round)
			clean = true
			break
		}

		fmt.Printf("REVIEW: markers found on round %d, running Addresser...\n", round)

		// Addresser step — reuse existing AddresserSystemPrompt.
		addresserPrompt := agent.AddresserSystemPrompt(opts.EpicTitle, opts.ContextFiles)
		addresserUser := fmt.Sprintf(
			"Address all REVIEW: markers in epic: %s\n\nEpic description:\n%s\n\n"+
				"Find all REVIEW: markers with grep, fix each issue, remove the marker, run tests.",
			opts.EpicTitle, opts.EpicBody,
		)
		if err := reviewAgentRun(agent.RunOptions{
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
			return fmt.Errorf("epic-holistic-review: re-check markers (round %d): %w", round, err)
		}
		if !hasMarkers {
			clean = true
			fmt.Printf("REVIEW: markers resolved after addressing on round %d.\n", round)
			break
		}
	}

	if clean {
		// Squash all review/address commits back into the work commit.
		squashMsg := fmt.Sprintf("feat(%s): %s", epicID, opts.EpicTitle)
		if err := reviewWorktreeSquash(wtPath, squashMsg); err != nil {
			fmt.Printf("Warning: could not squash review commits: %v\n", err)
		}
		fmt.Printf("Epic holistic review complete and clean for epic %s.\n", epicID)
	} else {
		fmt.Printf("REVIEW: markers remain after %d rounds for epic %s — human review needed.\n", opts.MaxRounds, epicID)
	}

	// Always transition to review-pending and notify.
	if err := reviewBeadSwapLabel(epicID, bead.LabelReviewPending); err != nil {
		fmt.Printf("Warning: could not label epic as review-pending: %v\n", err)
	}
	_ = opts.Notifier.Send(fmt.Sprintf("Sling: epic %q is ready for review. ID: %s", opts.EpicTitle, epicID))

	return nil
}
