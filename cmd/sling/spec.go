package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/aronasorman/sling/internal/config"
	"github.com/aronasorman/sling/internal/pipeline"
	"github.com/aronasorman/sling/internal/worktree"
)

var specCmd = &cobra.Command{
	Use:   "spec <bead-id>",
	Short: "Run the SpecAgent to write a technical specification for a bead",
	Long: `Run the SpecAgent (Sonnet) to produce a detailed technical specification for
the given bead. The spec is written to /tmp/sling-spec-<bead-id>.md and covers:
  - Implementation approach
  - Interface / API contracts
  - Test plan
  - Acceptance criteria

This command is also run automatically by 'sling next' before invoking the Executor.`,
	Args: cobra.ExactArgs(1),
	RunE: runSpec,
}

func init() {
	rootCmd.AddCommand(specCmd)
}

func runSpec(cmd *cobra.Command, args []string) error {
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
	contextFiles := loadContextFiles(cfg, repoRoot)

	_, err = pipeline.RunSpecAgent(beadID, repoRoot, contextFiles)
	if err != nil {
		return fmt.Errorf("spec: %w", err)
	}

	specFile := pipeline.SpecFile(beadID)
	fmt.Printf("Spec written to: %s\n", specFile)
	return nil
}
