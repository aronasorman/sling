package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/aronasorman/sling/internal/config"
	"github.com/aronasorman/sling/internal/notify"
	"github.com/aronasorman/sling/internal/pipeline"
	"github.com/aronasorman/sling/internal/worktree"
)

var nextCmd = &cobra.Command{
	Use:   "next",
	Short: "Claim the next sling:ready bead and execute it in a jj worktree",
	Args:  cobra.NoArgs,
	RunE:  runNext,
}

func init() {
	rootCmd.AddCommand(nextCmd)
}

func runNext(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	repoRoot := worktree.DetectRepoRoot(cwd)

	notifier := notify.New(
		cfg.Notify.TelegramEnabled,
		os.Getenv("SLING_TELEGRAM_TOKEN"),
		cfg.Notify.TelegramChatID,
	)

	contextFiles := loadContextFiles(cfg, repoRoot)

	result, err := pipeline.Execute(pipeline.ExecuteOptions{
		RepoRoot:        repoRoot,
		MaxAttempts:     cfg.Execution.MaxAttempts,
		ReviewMaxRounds: cfg.Execution.ReviewMaxRounds,
		SpecMaxTurns:    cfg.Execution.SpecMaxTurns,
		Notifier:        notifier,
		ContextFiles:    contextFiles,
	})
	if err != nil {
		return fmt.Errorf("execute: %w", err)
	}

	if !result.Succeeded {
		if result.BeadID == "" {
			return nil // nothing to do
		}
		return fmt.Errorf("bead %s failed", result.BeadID)
	}

	fmt.Printf("Bead %s is now sling:review-pending.\n", result.BeadID)
	return nil
}
