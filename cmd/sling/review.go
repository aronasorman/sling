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

var reviewCmd = &cobra.Command{
	Use:   "review <bead-id>",
	Short: "Run automated Sonnet review on a bead's diff, adding REVIEW: markers",
	Args:  cobra.ExactArgs(1),
	RunE:  runReview,
}

func init() {
	rootCmd.AddCommand(reviewCmd)
}

func runReview(cmd *cobra.Command, args []string) error {
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
		return fmt.Errorf("review: fetch bead: %w", err)
	}

	wtPath := b.WorktreePath
	if wtPath == "" {
		wtPath = worktree.WorktreePath(worktree.DetectRepoRoot(cwd), beadID)
	}

	contextFiles := loadContextFiles(cfg, worktree.DetectRepoRoot(cwd))

	fmt.Printf("Running Reviewer (Sonnet) for bead %s — %s\n", beadID, b.Title)
	systemPrompt := agent.ReviewerSystemPrompt(b.Title, contextFiles)
	userPrompt := fmt.Sprintf(
		"Review the implementation of bead: %s\n\nBead description:\n%s\n\n"+
			"Run `jj diff` first to see the changes, then add REVIEW: markers for any issues you find.",
		b.Title, b.Body,
	)

	if err := agent.Run(agent.RunOptions{
		WorkDir:      wtPath,
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Model:        agent.ModelSonnet,
		MaxTurns:     30,
	}); err != nil {
		return fmt.Errorf("review: agent: %w", err)
	}

	fmt.Printf("Review complete for bead %s.\n", beadID)
	return nil
}
