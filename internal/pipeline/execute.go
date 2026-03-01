package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aronasorman/sling/internal/agent"
	"github.com/aronasorman/sling/internal/bead"
	"github.com/aronasorman/sling/internal/notify"
	"github.com/aronasorman/sling/internal/worktree"
)

// ExecuteOptions configures the execution pipeline.
type ExecuteOptions struct {
	RepoRoot        string
	MaxAttempts     int
	ReviewMaxRounds int
	// SpecMaxTurns caps the number of agentic turns for the SpecAgent (0 → default 20).
	SpecMaxTurns int
	Notifier     *notify.Notifier
	ContextFiles map[string]string
}

// ExecuteResult is returned after execution completes.
type ExecuteResult struct {
	BeadID    string
	WtPath    string
	Succeeded bool
}

// ClaimNextReady finds the next sling:ready bead whose dependencies are all
// closed and returns it. Returns nil, nil if no bead is ready.
func ClaimNextReady() (*bead.Bead, error) {
	readyBeads, err := bead.List(bead.LabelReady)
	if err != nil {
		return nil, fmt.Errorf("execute: list ready beads: %w", err)
	}

	for _, b := range readyBeads {
		allClosed, err := allDepsClosed(b.DependsOn)
		if err != nil {
			return nil, err
		}
		if allClosed {
			return b, nil
		}
	}
	return nil, nil
}

// Execute claims the next ready bead, spawns the Executor agent in a jj worktree,
// and waits for the agent to call `sling signal-done`. On failure, labels the bead sling:failed.
func Execute(opts ExecuteOptions) (*ExecuteResult, error) {
	b, err := ClaimNextReady()
	if err != nil {
		return nil, err
	}
	if b == nil {
		fmt.Println("No sling:ready beads with all dependencies closed.")
		return &ExecuteResult{Succeeded: false}, nil
	}

	fmt.Printf("Claiming bead: %s — %s\n", b.ID, b.Title)

	// Atomically claim the bead and swap sling:ready → sling:executing in one
	// bd update call, preventing a window where the bead is claimed but still
	// carries the old label.
	if err := bead.ClaimAndLabel(b.ID, bead.LabelExecuting, bead.LabelReady); err != nil {
		return nil, fmt.Errorf("execute: claim bead %s: %w", b.ID, err)
	}

	// Create jj worktree.
	wt, err := worktree.Add(opts.RepoRoot, b.ID)
	if err != nil {
		return nil, fmt.Errorf("execute: create worktree: %w", err)
	}
	fmt.Printf("Worktree: %s\n", wt.Path)

	// Record worktree path on bead.
	if err := bead.UpdateWorktree(b.ID, wt.Path); err != nil {
		// Non-fatal: just log.
		fmt.Printf("Warning: could not record worktree path on bead: %v\n", err)
	}

	// Run spec agent before executor so the Executor has a detailed technical spec.
	executorContextFiles := opts.ContextFiles
	specContent, specErr := RunSpecAgent(b.ID, opts.RepoRoot, opts.ContextFiles, opts.SpecMaxTurns)
	if specErr != nil {
		fmt.Printf("Warning: spec agent failed (proceeding without spec): %v\n", specErr)
	} else if specContent != "" {
		// Merge spec into a fresh map so we don't mutate the shared options.
		enriched := make(map[string]string, len(opts.ContextFiles)+1)
		for k, v := range opts.ContextFiles {
			enriched[k] = v
		}
		// Use the reserved "__spec__" key so user-configured context file names
		// (from sling.toml) can never collide with the generated spec.
		enriched["__spec__"] = specContent
		executorContextFiles = enriched
	}

	// Run executor with retry.
	if opts.MaxAttempts < 1 {
		opts.MaxAttempts = 3
	}

	var lastErr error
	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		fmt.Printf("Executor attempt %d/%d for bead %s\n", attempt, opts.MaxAttempts, b.ID)

		systemPrompt := agent.ExecutorSystemPrompt(b.Title, b.Body, b.ID, executorContextFiles)
		userPrompt := fmt.Sprintf("Implement bead: %s\n\n%s", b.Title, b.Body)
		if attempt >= 2 {
			userPrompt = fmt.Sprintf("⚠️ RETRY ATTEMPT %d: A previous attempt did not complete successfully. Implement the bead, ensure all tests pass, commit your changes, then run `sling signal-done %s`.\n\n", attempt, b.ID) + userPrompt
		}

		err := agent.Run(agent.RunOptions{
			WorkDir:      wt.Path,
			SystemPrompt: systemPrompt,
			UserPrompt:   userPrompt,
			Model:        agent.ModelSonnet,
			MaxTurns:     50,
			Env:          map[string]string{"SLING_BEAD_ID": b.ID},
		})
		if err != nil {
			lastErr = err
			fmt.Printf("Agent exited with error on attempt %d: %v\n", attempt, err)
			continue
		}

		// Agent returned successfully — treat as success.
		fmt.Printf("Bead %s completed successfully.\n", b.ID)
		// Run automated review before labeling review-pending.
		// RunAutomatedReview handles label + notification when it finishes.
		if err := RunAutomatedReview(b.ID, wt.Path, AutoReviewOptions{
			MaxRounds:    opts.ReviewMaxRounds,
			Notifier:     opts.Notifier,
			ContextFiles: opts.ContextFiles,
		}); err != nil {
			fmt.Printf("Warning: automated review error: %v\n", err)
		}
		return &ExecuteResult{BeadID: b.ID, WtPath: wt.Path, Succeeded: true}, nil
	}

	// All attempts failed.
	fmt.Printf("Bead %s failed after %d attempts: %v\n", b.ID, opts.MaxAttempts, lastErr)
	if err := bead.RemoveLabel(b.ID, bead.LabelExecuting); err != nil {
		fmt.Printf("Warning: could not remove executing label: %v\n", err)
	}
	if err := bead.AddLabel(b.ID, bead.LabelFailed); err != nil {
		fmt.Printf("Warning: could not add failed label: %v\n", err)
	}
	if err := bead.SetStatus(b.ID, bead.StatusBlocked); err != nil {
		fmt.Printf("Warning: could not set status to blocked: %v\n", err)
	}
	_ = opts.Notifier.Send(fmt.Sprintf("Sling: bead %q FAILED after %d attempts. ID: %s", b.Title, opts.MaxAttempts, b.ID))

	return &ExecuteResult{BeadID: b.ID, WtPath: wt.Path, Succeeded: false}, lastErr
}

// HasReviewMarkers returns true if any file in dir contains a REVIEW: marker.
// Handles Go, Python, SQL, HTML, JS comment styles.
// Per issue #6: done = no REVIEW: markers, not empty diff.
func HasReviewMarkers(dir string) (bool, error) {
	// Marker patterns per ARCHITECTURE.md.
	markers := []string{"# REVIEW:", "// REVIEW:", "-- REVIEW:", "<!-- REVIEW:", "/* REVIEW:"}

	// Extensions where REVIEW: might legitimately appear as prose (not code markers).
	skipExts := map[string]bool{".md": true, ".txt": true, ".rst": true}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir // skip hidden dirs like .git, .jj
			}
			return nil
		}
		if skipExts[strings.ToLower(filepath.Ext(path))] {
			return nil // skip prose/doc files to avoid false positives
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}
		content := string(data)
		for _, marker := range markers {
			if strings.Contains(content, marker) {
				return fmt.Errorf("found") // abuse error to short-circuit
			}
		}
		return nil
	})

	if err != nil && err.Error() == "found" {
		return true, nil
	}
	return false, err
}
