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

var (
	nextEpicID string
	nextLoop   bool
)

var nextCmd = &cobra.Command{
	Use:   "next",
	Short: "Claim the next sling:ready bead and execute it in a jj worktree",
	Args:  cobra.NoArgs,
	RunE:  runNext,
}

func init() {
	rootCmd.AddCommand(nextCmd)
	nextCmd.Flags().StringVar(&nextEpicID, "epic", "", "Restrict execution to beads of this epic ID")
	nextCmd.Flags().BoolVar(&nextLoop, "loop", false, "Keep executing beads until none remain")
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

	opts := pipeline.ExecuteOptions{
		RepoRoot:        repoRoot,
		MaxAttempts:     cfg.Execution.MaxAttempts,
		ReviewMaxRounds: cfg.Execution.ReviewMaxRounds,
		SpecMaxTurns:    cfg.Execution.SpecMaxTurns,
		Notifier:        notifier,
		ContextFiles:    contextFiles,
		EpicID:          nextEpicID,
	}

	result, err := pipeline.ClaimAndExecute(opts)
	if err != nil {
		return fmt.Errorf("execute: %w", err)
	}

	if !result.Succeeded {
		// Nothing to do (no ready beads).
		if result.BeadID == "" && result.EpicID == "" {
			return nil
		}
		if result.IsEpic {
			return fmt.Errorf("epic %s failed", result.EpicID)
		}
		return fmt.Errorf("bead %s failed", result.BeadID)
	}

	if result.IsEpic {
		fmt.Printf("Epic %s is now sling:review-pending.\n", result.EpicID)
	} else {
		fmt.Printf("Bead %s is now sling:review-pending.\n", result.BeadID)
	}
	return nil
}
