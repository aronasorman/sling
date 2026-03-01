package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/aronasorman/sling/internal/bead"
	"github.com/aronasorman/sling/internal/config"
	"github.com/aronasorman/sling/internal/pipeline"
	"github.com/aronasorman/sling/internal/worktree"
)

var doneCmd = &cobra.Command{
	Use:   "done <bead-id>",
	Short: "Squash worktree commits, push branch, close bead. No autonomous merging.",
	Args:  cobra.ExactArgs(1),
	RunE:  runDone,
}

func init() {
	rootCmd.AddCommand(doneCmd)
}

func runDone(cmd *cobra.Command, args []string) error {
	beadID := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	_, err = config.Load(cwd)
	if err != nil {
		return err
	}

	b, err := bead.Show(beadID)
	if err != nil {
		return fmt.Errorf("done: fetch bead: %w", err)
	}

	wtPath := b.WorktreePath
	if wtPath == "" {
		wtPath = worktree.WorktreePath(worktree.DetectRepoRoot(cwd), beadID)
	}

	// Issue #6: check that no REVIEW: markers remain before allowing done.
	hasMarkers, err := pipeline.HasReviewMarkers(wtPath)
	if err != nil {
		return fmt.Errorf("done: check review markers: %w", err)
	}
	if hasMarkers {
		return fmt.Errorf("REVIEW: markers still exist in worktree %s — run `sling address %s` first", wtPath, beadID)
	}

	branch := "sling/" + beadID

	// Squash all commits into one.
	squashMsg := fmt.Sprintf("feat(%s): %s", beadID, b.Title)
	if err := worktree.Squash(wtPath, squashMsg); err != nil {
		return fmt.Errorf("done: squash: %w", err)
	}

	// Push branch (no merge).
	if err := worktree.PushBranch(wtPath, branch, "origin"); err != nil {
		return fmt.Errorf("done: push branch: %w", err)
	}

	// Close the bead.
	if err := bead.SetStatus(beadID, bead.StatusClosed); err != nil {
		return fmt.Errorf("done: close bead: %w", err)
	}

	// Remove all sling: labels.
	for _, label := range []string{
		bead.LabelReviewPending, bead.LabelAddressing, bead.LabelExecuting,
		bead.LabelPlanned, bead.LabelReady, bead.LabelFailed, bead.LabelBlocked,
	} {
		_ = bead.RemoveLabel(beadID, label) // best-effort
	}

	fmt.Printf("Bead %s is done. Branch %q pushed. Open a PR and merge manually.\n", beadID, branch)

	// Promote any beads that now have all deps closed.
	parentID := b.ParentID
	if parentID != "" {
		if err := pipeline.PromoteReadyBeads(parentID); err != nil {
			fmt.Printf("Warning: could not promote ready beads: %v\n", err)
		}
	}

	return nil
}
