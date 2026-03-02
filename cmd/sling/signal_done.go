package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aronasorman/sling/internal/bead"
	"github.com/aronasorman/sling/internal/pipeline"
)

var signalDoneCmd = &cobra.Command{
	Use:   "signal-done <bead-id>",
	Short: "Manual rescue only: mark a bead as review-pending (orchestrator does this automatically).",
	Args:  cobra.ExactArgs(1),
	RunE:  runSignalDone,
}

func init() {
	rootCmd.AddCommand(signalDoneCmd)
}

func runSignalDone(cmd *cobra.Command, args []string) error {
	beadID := args[0]

	b, err := bead.Show(beadID)
	if err != nil {
		return fmt.Errorf("signal-done: fetch bead: %w", err)
	}

	// Sub-bead: redirect to the epic.
	if b.ParentID != "" {
		return fmt.Errorf(
			"bead %s is a sub-bead of epic %s — run: sling signal-done %s",
			beadID, b.ParentID, b.ParentID,
		)
	}

	// Epic bead: transition epic + all executing sub-beads.
	subBeads, err := pipeline.ListSubBeads(beadID)
	if err != nil {
		return fmt.Errorf("signal-done: list sub-beads: %w", err)
	}
	if len(subBeads) > 0 {
		return pipeline.SignalDoneEpic(beadID)
	}

	// Standalone bead: existing behaviour.
	if err := bead.RemoveLabel(beadID, bead.LabelExecuting); err != nil {
		return fmt.Errorf("signal-done: remove executing label: %w", err)
	}
	if err := bead.AddLabel(beadID, bead.LabelReviewPending); err != nil {
		return fmt.Errorf("signal-done: add review-pending label: %w", err)
	}
	fmt.Printf("✓ Bead %s marked review-pending\n", beadID)
	return nil
}
