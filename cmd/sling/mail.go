package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aronasorman/sling/internal/bead"
)

var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Show a digest of beads that need attention",
	Args:  cobra.NoArgs,
	RunE:  runMail,
}

func init() {
	rootCmd.AddCommand(mailCmd)
}

func runMail(cmd *cobra.Command, args []string) error {
	// Collect beads in states that need human attention.
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
