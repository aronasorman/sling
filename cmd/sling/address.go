package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/aronasorman/sling/internal/agent"
	"github.com/aronasorman/sling/internal/bead"
	"github.com/aronasorman/sling/internal/config"
	"github.com/aronasorman/sling/internal/worktree"
)

var addressCmd = &cobra.Command{
	Use:   "address <bead-id>",
	Short: "Run the Addresser agent to resolve all REVIEW: markers in a bead's worktree",
	Args:  cobra.ExactArgs(1),
	RunE:  runAddress,
}

func init() {
	rootCmd.AddCommand(addressCmd)
}

func runAddress(cmd *cobra.Command, args []string) error {
	beadID := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	b, err := bead.Show(beadID)
	if err != nil {
		return fmt.Errorf("address: fetch bead: %w", err)
	}

	wtPath := b.WorktreePath
	if wtPath == "" {
		wtPath = worktree.WorktreePath(worktree.DetectRepoRoot(cwd), beadID)
	}

	// Transition to addressing.
	if err := bead.RemoveLabel(beadID, bead.LabelReviewPending); err != nil {
		fmt.Printf("Warning: could not remove review-pending label: %v\n", err)
	}
	if err := bead.AddLabel(beadID, bead.LabelAddressing); err != nil {
		fmt.Printf("Warning: could not add addressing label: %v\n", err)
	}

	contextFiles := loadContextFiles(cfg, worktree.DetectRepoRoot(cwd))

	fmt.Printf("Running Addresser (Sonnet) for bead %s — %s\n", beadID, b.Title)
	systemPrompt := agent.AddresserSystemPrompt(b.Title, contextFiles)
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
