package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/aronasorman/sling/internal/config"
	"github.com/aronasorman/sling/internal/pipeline"
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

	repoRoot := worktree.DetectRepoRoot(cwd)
	return pipeline.Review(beadID, pipeline.ReviewOptions{
		RepoRoot:     repoRoot,
		ContextFiles: loadContextFiles(cfg, repoRoot),
	})
}
