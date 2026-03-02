package pipeline

import (
	"fmt"
	"strings"

	"github.com/aronasorman/sling/internal/agent"
	"github.com/aronasorman/sling/internal/bead"
	"github.com/aronasorman/sling/internal/worktree"
)

// Injectable function variables for human.go (overridden in tests).
var (
	humanBeadShow           = func(id string) (*bead.Bead, error) { return bead.Show(id) }
	humanListSubBeads       = func(epicID string) ([]*bead.Bead, error) { return ListSubBeads(epicID) }
	humanHasReviewMarkers   = func(dir string) (bool, error) { return HasReviewMarkers(dir) }
	humanWorktreeSquash     = func(wtPath, msg string) error { return worktree.Squash(wtPath, msg) }
	humanWorktreePushBranch = func(wtPath, branch, remote string) error {
		return worktree.PushBranch(wtPath, branch, remote)
	}
	humanWorktreeRemove   = func(repoRoot, id string) error { return worktree.Remove(repoRoot, id) }
	humanBeadSetStatus    = func(id, status string) error { return bead.SetStatus(id, status) }
	humanBeadRemoveLabel  = func(id, label string) error { return bead.RemoveLabel(id, label) }
	humanDoneEpicFn       = func(epicID, repoRoot string) error { return DoneEpic(epicID, repoRoot) }
	humanWorktreePathFn   = func(repoRoot, id string) string { return worktree.WorktreePath(repoRoot, id) }
	humanWorktreePathFromBead = func(b *bead.Bead) string { return bead.WorktreePathFromBead(b) }
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
// For sub-beads it redirects to the parent epic; for epics it calls DoneEpic.
func Done(beadID, repoRoot string) error {
	b, err := humanBeadShow(beadID)
	if err != nil {
		return fmt.Errorf("done: fetch bead: %w", err)
	}

	// Sub-bead: redirect caller to the epic.
	if b.ParentID != "" {
		return fmt.Errorf("bead %s is a sub-bead of epic %s — run: sling done %s", beadID, b.ParentID, b.ParentID)
	}

	// Epic bead (has sub-beads): route to DoneEpic.
	subBeads, err := humanListSubBeads(beadID)
	if err != nil {
		return fmt.Errorf("done: list sub-beads: %w", err)
	}
	if len(subBeads) > 0 {
		return humanDoneEpicFn(beadID, repoRoot)
	}

	// Standalone bead: original logic.
	wtPath := humanWorktreePathFromBead(b)
	if wtPath == "" {
		wtPath = humanWorktreePathFn(repoRoot, beadID)
	}

	hasMarkers, err := humanHasReviewMarkers(wtPath)
	if err != nil {
		return fmt.Errorf("done: check review markers: %w", err)
	}
	if hasMarkers {
		return fmt.Errorf("REVIEW: markers still exist in worktree %s — run `sling address %s` first", wtPath, beadID)
	}

	branch := "sling/" + beadID
	squashMsg := fmt.Sprintf("feat(%s): %s", beadID, b.Title)
	if err := humanWorktreeSquash(wtPath, squashMsg); err != nil {
		return fmt.Errorf("done: squash: %w", err)
	}

	if err := humanWorktreePushBranch(wtPath, branch, "origin"); err != nil {
		return fmt.Errorf("done: push branch: %w", err)
	}

	if err := humanBeadSetStatus(beadID, bead.StatusClosed); err != nil {
		return fmt.Errorf("done: close bead: %w", err)
	}

	// Remove all sling: labels (best-effort).
	for _, label := range []string{
		bead.LabelReviewPending, bead.LabelAddressing, bead.LabelExecuting,
		bead.LabelPlanned, bead.LabelReady, bead.LabelFailed, bead.LabelBlocked,
	} {
		_ = humanBeadRemoveLabel(beadID, label)
	}

	fmt.Printf("Bead %s is done. Branch %q pushed. Open a PR and merge manually.\n", beadID, branch)

	return nil
}

// DoneEpic squashes, pushes, and closes an epic together with all its sub-beads.
// It refuses if any sub-bead is not in sling:review-pending or closed, or if
// REVIEW: markers remain in the epic worktree.
func DoneEpic(epicID, repoRoot string) error {
	epicBead, err := humanBeadShow(epicID)
	if err != nil {
		return fmt.Errorf("done-epic: fetch epic bead: %w", err)
	}
	if epicBead.ParentID != "" {
		return fmt.Errorf("done-epic: bead %s is a sub-bead (ParentID=%s) — run: sling done %s", epicID, epicBead.ParentID, epicBead.ParentID)
	}

	subBeads, err := humanListSubBeads(epicID)
	if err != nil {
		return fmt.Errorf("done-epic: list sub-beads: %w", err)
	}

	// Validate all sub-beads are review-pending or already closed.
	for _, sb := range subBeads {
		if sb.Status == bead.StatusClosed {
			continue
		}
		if bead.HasLabel(sb, bead.LabelReviewPending) {
			continue
		}
		return fmt.Errorf("done-epic: sub-bead %s is in state %v — not ready for done", sb.ID, sb.Labels)
	}

	// Determine worktree path.
	wtPath := humanWorktreePathFromBead(epicBead)
	if wtPath == "" {
		wtPath = humanWorktreePathFn(repoRoot, epicID)
	}

	// Refuse if REVIEW: markers remain.
	hasMarkers, err := humanHasReviewMarkers(wtPath)
	if err != nil {
		return fmt.Errorf("done-epic: check review markers: %w", err)
	}
	if hasMarkers {
		return fmt.Errorf("REVIEW: markers still exist in worktree %s — run `sling address %s` first", wtPath, epicID)
	}

	branch := "sling/" + epicID
	squashMsg := fmt.Sprintf("feat(%s): %s", epicID, epicBead.Title)
	if err := humanWorktreeSquash(wtPath, squashMsg); err != nil {
		return fmt.Errorf("done-epic: squash: %w", err)
	}

	if err := humanWorktreePushBranch(wtPath, branch, "origin"); err != nil {
		return fmt.Errorf("done-epic: push branch: %w", err)
	}

	// Close each sub-bead that is not already closed.
	allSlingLabels := []string{
		bead.LabelReviewPending, bead.LabelAddressing, bead.LabelExecuting,
		bead.LabelPlanned, bead.LabelReady, bead.LabelFailed, bead.LabelBlocked,
	}
	for _, sb := range subBeads {
		if sb.Status == bead.StatusClosed {
			continue
		}
		if err := humanBeadSetStatus(sb.ID, bead.StatusClosed); err != nil {
			fmt.Printf("Warning: could not close sub-bead %s: %v\n", sb.ID, err)
		}
		for _, label := range allSlingLabels {
			_ = humanBeadRemoveLabel(sb.ID, label)
		}
	}

	// Close the epic.
	if err := humanBeadSetStatus(epicID, bead.StatusClosed); err != nil {
		return fmt.Errorf("done-epic: close epic bead: %w", err)
	}
	for _, label := range allSlingLabels {
		_ = humanBeadRemoveLabel(epicID, label)
	}

	// Clean up the shared worktree.
	if err := humanWorktreeRemove(repoRoot, epicID); err != nil {
		fmt.Printf("Warning: could not remove worktree for epic %s: %v\n", epicID, err)
	}

	fmt.Printf("Epic %s is done. Branch %q pushed. Open a PR and merge manually.\n", epicID, branch)
	return nil
}
