package pipeline

import (
	"fmt"
	"strings"

	"github.com/aronasorman/sling/internal/agent"
	"github.com/aronasorman/sling/internal/bead"
	"github.com/aronasorman/sling/internal/worktree"
)

// Mail prints a digest of beads that need human attention, grouped by label.
func Mail() error {
	type section struct {
		label string
		title string
	}
	sections := []section{
		{bead.LabelReviewPending, "Review Pending"},
		{bead.LabelFailed, "Failed"},
		{bead.LabelBlocked, "Blocked"},
		{bead.LabelAddressing, "Addressing (in progress)"},
		{bead.LabelExecuting, "Executing (in progress)"},
		{bead.LabelReady, "Ready (waiting to run)"},
		{bead.LabelPlanned, "Planned"},
	}

	any := false
	for _, s := range sections {
		beads, err := bead.List(s.label)
		if err != nil {
			return fmt.Errorf("mail: list %s: %w", s.label, err)
		}
		if len(beads) == 0 {
			continue
		}
		any = true
		fmt.Printf("\n## %s (%d)\n", s.title, len(beads))
		fmt.Println(strings.Repeat("-", 40))
		for _, b := range beads {
			fmt.Printf("  %s  %s\n", b.ID, b.Title)
		}
	}

	if !any {
		fmt.Println("Nothing needs your attention. Run `sling next` to process the next ready bead.")
	}
	return nil
}

// ReviewOptions configures a review pipeline run.
type ReviewOptions struct {
	RepoRoot     string
	ContextFiles map[string]string
}

// Review creates a new jj commit in the bead's worktree so a human reviewer
// can add REVIEW: markers directly in the code files.
// After adding markers, run `sling address <id>` to have the Addresser resolve them.
func Review(beadID string, opts ReviewOptions) error {
	b, err := bead.Show(beadID)
	if err != nil {
		return fmt.Errorf("review: fetch bead: %w", err)
	}

	wtPath := bead.WorktreePathFromBead(b)
	if wtPath == "" {
		wtPath = worktree.WorktreePath(opts.RepoRoot, beadID)
	}

	commitMsg := fmt.Sprintf("review: %s", beadID)
	if err := worktree.NewCommit(wtPath, commitMsg); err != nil {
		return fmt.Errorf("review: create review commit: %w", err)
	}

	fmt.Printf("Review commit created in worktree: %s\n", wtPath)
	fmt.Printf("Add REVIEW: markers to code files, then run `sling address %s` to resolve them.\n", beadID)
	return nil
}

// AddressOptions configures an address pipeline run.
type AddressOptions struct {
	RepoRoot     string
	ContextFiles map[string]string
}

// Address runs the Addresser agent (Sonnet) to resolve all REVIEW: markers in a bead's worktree.
func Address(beadID string, opts AddressOptions) error {
	b, err := bead.Show(beadID)
	if err != nil {
		return fmt.Errorf("address: fetch bead: %w", err)
	}

	wtPath := bead.WorktreePathFromBead(b)
	if wtPath == "" {
		wtPath = worktree.WorktreePath(opts.RepoRoot, beadID)
	}

	// Transition to addressing.
	if err := bead.RemoveLabel(beadID, bead.LabelReviewPending); err != nil {
		fmt.Printf("Warning: could not remove review-pending label: %v\n", err)
	}
	if err := bead.AddLabel(beadID, bead.LabelAddressing); err != nil {
		fmt.Printf("Warning: could not add addressing label: %v\n", err)
	}

	fmt.Printf("Running Addresser (Sonnet) for bead %s — %s\n", beadID, b.Title)
	systemPrompt := agent.AddresserSystemPrompt(b.Title, opts.ContextFiles)
	userPrompt := fmt.Sprintf(
		"Address all REVIEW: markers in bead: %s\n\nBead description:\n%s\n\n"+
			"Find all REVIEW: markers with grep, fix each issue, remove the marker, run tests.",
		b.Title, b.Body,
	)

	if err := agent.Run(agent.RunOptions{
		WorkDir:      wtPath,
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Model:        agent.ModelSonnet,
		MaxTurns:     40,
	}); err != nil {
		return fmt.Errorf("address: agent: %w", err)
	}

	// Transition back to review-pending.
	if err := bead.RemoveLabel(beadID, bead.LabelAddressing); err != nil {
		fmt.Printf("Warning: could not remove addressing label: %v\n", err)
	}
	if err := bead.AddLabel(beadID, bead.LabelReviewPending); err != nil {
		fmt.Printf("Warning: could not add review-pending label: %v\n", err)
	}

	fmt.Printf("Addressing complete for bead %s. Run `sling review %s` to re-check.\n", beadID, beadID)
	return nil
}

// Done squashes all worktree commits into one, pushes the branch, and closes the bead.
// It refuses to proceed if REVIEW: markers remain in the worktree (issue #6).
func Done(beadID, repoRoot string) error {
	b, err := bead.Show(beadID)
	if err != nil {
		return fmt.Errorf("done: fetch bead: %w", err)
	}

	wtPath := bead.WorktreePathFromBead(b)
	if wtPath == "" {
		wtPath = worktree.WorktreePath(repoRoot, beadID)
	}

	hasMarkers, err := HasReviewMarkers(wtPath)
	if err != nil {
		return fmt.Errorf("done: check review markers: %w", err)
	}
	if hasMarkers {
		return fmt.Errorf("REVIEW: markers still exist in worktree %s — run `sling address %s` first", wtPath, beadID)
	}

	branch := "sling/" + beadID
	squashMsg := fmt.Sprintf("feat(%s): %s", beadID, b.Title)
	if err := worktree.Squash(wtPath, squashMsg); err != nil {
		return fmt.Errorf("done: squash: %w", err)
	}

	if err := worktree.PushBranch(wtPath, branch, "origin"); err != nil {
		return fmt.Errorf("done: push branch: %w", err)
	}

	if err := bead.SetStatus(beadID, bead.StatusClosed); err != nil {
		return fmt.Errorf("done: close bead: %w", err)
	}

	// Remove all sling: labels (best-effort).
	for _, label := range []string{
		bead.LabelReviewPending, bead.LabelAddressing, bead.LabelExecuting,
		bead.LabelPlanned, bead.LabelReady, bead.LabelFailed, bead.LabelBlocked,
	} {
		_ = bead.RemoveLabel(beadID, label)
	}

	fmt.Printf("Bead %s is done. Branch %q pushed. Open a PR and merge manually.\n", beadID, branch)

	// Promote any beads that now have all deps closed.
	if b.ParentID != "" {
		if err := PromoteReadyBeads(b.ParentID); err != nil {
			fmt.Printf("Warning: could not promote ready beads: %v\n", err)
		}
	}

	return nil
}
