package main

import (
	"os"

	"github.com/spf13/cobra"

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

	repoRoot := worktree.DetectRepoRoot(cwd)
	return pipeline.Done(beadID, repoRoot)
}
