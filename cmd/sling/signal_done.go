package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aronasorman/sling/internal/bead"
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
	if beadID == "" {
		return fmt.Errorf("bead-id is required")
	}

	if err := bead.RemoveLabel(beadID, bead.LabelExecuting); err != nil {
		return fmt.Errorf("signal-done: remove executing label: %w", err)
	}
	if err := bead.AddLabel(beadID, bead.LabelReviewPending); err != nil {
		return fmt.Errorf("signal-done: add review-pending label: %w", err)
	}

	fmt.Printf("✓ Bead %s marked review-pending\n", beadID)
	return nil
}
