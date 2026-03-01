package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/aronasorman/sling/internal/config"
	"github.com/aronasorman/sling/internal/pipeline"
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

	repoRoot := worktree.DetectRepoRoot(cwd)
	return pipeline.Address(beadID, pipeline.AddressOptions{
		RepoRoot:     repoRoot,
		ContextFiles: loadContextFiles(cfg, repoRoot),
	})
}
