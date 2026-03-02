package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aronasorman/sling/internal/agent"
	"github.com/aronasorman/sling/internal/bead"
	"github.com/aronasorman/sling/internal/notify"
	"github.com/aronasorman/sling/internal/worktree"
)

// BuildGateConfig configures the build and test gate.
type BuildGateConfig struct {
	// BuildCmd is the shell command run first. Empty = skip.
	BuildCmd string
	// TestCmd is the shell command run after a successful build. Empty = skip.
	TestCmd string
	// Timeout is the per-command timeout. Zero means 10 minutes.
	Timeout time.Duration
}

// BuildGateResult holds the outcome of RunBuildGate.
type BuildGateResult struct {
	// BuildPassed is true when BuildCmd exited 0 (or was skipped).
	BuildPassed bool
	// TestPassed is true when TestCmd exited 0 (or was skipped).
	TestPassed bool
	// BuildOutput is the combined stdout+stderr of BuildCmd.
	BuildOutput string
	// TestOutput is the combined stdout+stderr of TestCmd.
	TestOutput string
	// Skipped is true when both BuildCmd and TestCmd are empty strings.
	Skipped bool
}

// RunBuildGate runs the build and test commands inside wtPath.
// It inherits the full environment of the current process (including PATH)
// so that tools like go, npm, etc. are found without explicit configuration.
func RunBuildGate(wtPath string, cfg BuildGateConfig) (*BuildGateResult, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Minute
	}

	result := &BuildGateResult{}

	if cfg.BuildCmd == "" && cfg.TestCmd == "" {
		result.Skipped = true
		result.BuildPassed = true
		result.TestPassed = true
		return result, nil
	}

	if cfg.BuildCmd == "" {
		result.BuildPassed = true
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, "sh", "-c", cfg.BuildCmd)
		cmd.Dir = wtPath
		cmd.Env = os.Environ()
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		err := cmd.Run()
		result.BuildOutput = buf.String()
		if err != nil {
			result.BuildPassed = false
			if ctx.Err() != nil {
				return nil, fmt.Errorf("run build command %q: timed out after %s", cfg.BuildCmd, cfg.Timeout)
			}
			return result, nil
		}
		result.BuildPassed = true
	}

	if cfg.TestCmd == "" {
		result.TestPassed = true
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, "sh", "-c", cfg.TestCmd)
		cmd.Dir = wtPath
		cmd.Env = os.Environ()
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		err := cmd.Run()
		result.TestOutput = buf.String()
		if err != nil {
			result.TestPassed = false
			if ctx.Err() != nil {
				return nil, fmt.Errorf("run test command %q: timed out after %s", cfg.TestCmd, cfg.Timeout)
			}
			return result, nil
		}
		result.TestPassed = true
	}

	return result, nil
}

// executeBeadList is an injectable stub for bead.List used by ClaimNextReady.
var executeBeadList = func(label string) ([]*bead.Bead, error) {
	return bead.List(label)
}

// ExecuteOptions configures the execution pipeline.
type ExecuteOptions struct {
	RepoRoot        string
	MaxAttempts     int
	ReviewMaxRounds int
	// SpecMaxTurns caps the number of agentic turns for the SpecAgent (0 → default 20).
	SpecMaxTurns int
	Notifier     *notify.Notifier
	ContextFiles map[string]string
	// EpicID when non-empty restricts ClaimNextReady to beads with ParentID == EpicID.
	EpicID string
}

// ExecuteResult is returned after execution completes.
type ExecuteResult struct {
	BeadID    string
	WtPath    string
	Succeeded bool
}

// ClaimNextReady finds the next sling:ready bead whose dependencies are all
// closed and returns it. When epicID is non-empty only beads whose ParentID
// matches are considered. Returns (nil, nil) when no eligible bead exists.
func ClaimNextReady(epicID string) (*bead.Bead, error) {
	readyBeads, err := executeBeadList(bead.LabelReady)
	if err != nil {
		return nil, fmt.Errorf("execute: list ready beads: %w", err)
	}

	for _, b := range readyBeads {
		if epicID != "" && b.ParentID != epicID {
			continue
		}
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
// and runs automated review. On failure, labels the bead sling:failed.
func Execute(opts ExecuteOptions) (*ExecuteResult, error) {
	b, err := ClaimNextReady(opts.EpicID)
	if err != nil {
		return nil, err
	}
	if b == nil {
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
			userPrompt = fmt.Sprintf("⚠️ RETRY ATTEMPT %d: A previous attempt did not complete successfully. Implement the bead, ensure all tests pass, and commit your changes.\n\n", attempt) + userPrompt
		}

		err := agent.Run(agent.RunOptions{
			WorkDir:      wt.Path,
			SystemPrompt: systemPrompt,
			UserPrompt:   userPrompt,
			Model:        agent.ModelSonnet,
			MaxTurns:     100,
			Env:          map[string]string{"SLING_BEAD_ID": b.ID},
		})
		if err != nil {
			lastErr = err
			fmt.Printf("Agent exited with error on attempt %d: %v\n", attempt, err)
			continue
		}

		// Agent returned successfully — run automated review before transitioning
		// the bead to sling:review-pending so the bead is not visible to `sling mail`
		// until the review pass is complete.
		// RunAutomatedReview handles the label swap (executing → review-pending) and
		// sends the notification when it finishes.
		fmt.Printf("Bead %s completed successfully.\n", b.ID)
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

// ─── Epic execution types ─────────────────────────────────────────────────────

// EpicExecuteOptions configures ExecuteEpic.
type EpicExecuteOptions struct {
	RepoRoot        string
	EpicID          string
	ReviewMaxRounds int
	// SpecMaxTurns caps the number of agentic turns for the SpecAgent (0 → default 20).
	SpecMaxTurns int
	Notifier     *notify.Notifier
	ContextFiles map[string]string
}

// EpicExecuteResult is returned by ExecuteEpic.
type EpicExecuteResult struct {
	EpicID    string
	WtPath    string
	BeadIDs   []string // sub-bead IDs that were claimed and executed
	Succeeded bool
}

// ClaimAndExecuteResult is returned by ClaimAndExecute.
type ClaimAndExecuteResult struct {
	IsEpic    bool
	EpicID    string // set when IsEpic == true
	BeadID    string // set when IsEpic == false
	Succeeded bool
}

// ─── Injectable function variables (overridden in tests) ─────────────────────

var (
	// Used by ListSubBeads, ExecuteEpic.
	execBeadList = func(label string) ([]*bead.Bead, error) { return bead.List(label) }
	// Used by ExecuteEpic, SignalDoneEpic.
	execBeadShow = func(id string) (*bead.Bead, error) { return bead.Show(id) }
	// Used by ExecuteEpic, SignalDoneEpic.
	execBeadSwapLabel = func(id, label string) error { return bead.SwapSlingLabel(id, label) }
	// Used by ExecuteEpic.
	execBeadClaimAndLabel  = func(id, add, remove string) error { return bead.ClaimAndLabel(id, add, remove) }
	execBeadUpdateWorktree = func(id, path string) error { return bead.UpdateWorktree(id, path) }
	execWorktreeAdd        = func(root, id string) (*worktree.Workspace, error) { return worktree.Add(root, id) }
	execAgentRun           = func(opts agent.RunOptions) error { return agent.Run(opts) }
	execRunSpecAgent       = func(beadID, repoRoot string, ctx map[string]string, maxTurns int) (string, error) {
		return RunSpecAgent(beadID, repoRoot, ctx, maxTurns)
	}
	execRunAutoReview = func(beadID, wtPath string, opts AutoReviewOptions) error {
		return RunAutomatedReview(beadID, wtPath, opts)
	}

	// Routing for ClaimAndExecute.
	execClaimNextReady  = func(epicID string) (*bead.Bead, error) { return ClaimNextReady(epicID) }
	execRouteEpic       = func(opts EpicExecuteOptions) (*EpicExecuteResult, error) { return ExecuteEpic(opts) }
	execRouteStandalone = func(opts ExecuteOptions) (*ExecuteResult, error) { return Execute(opts) }

	// Used by SignalDoneEpic to enumerate sub-beads.
	execListSubBeads = func(epicID string) ([]*bead.Bead, error) { return ListSubBeads(epicID) }
)

// ─── ListSubBeads ─────────────────────────────────────────────────────────────

// ListSubBeads returns all beads whose ParentID equals epicID.
// Returns an empty (non-nil) slice when none are found.
func ListSubBeads(epicID string) ([]*bead.Bead, error) {
	all, err := execBeadList("")
	if err != nil {
		return nil, fmt.Errorf("ListSubBeads: %w", err)
	}
	result := make([]*bead.Bead, 0)
	for _, b := range all {
		if b.ParentID == epicID {
			result = append(result, b)
		}
	}
	return result, nil
}

// ─── topoSortBeads ────────────────────────────────────────────────────────────

// topoSortBeads performs Kahn's topological sort on a slice of beads.
// Only dependencies whose IDs are present in the input slice are considered;
// external dependencies are silently ignored.
// Returns beads ordered so every bead appears after all its in-slice dependencies.
// Returns an error wrapping ErrCycle if a cycle exists among in-slice deps.
func topoSortBeads(beads []*bead.Bead) ([]*bead.Bead, error) {
	if len(beads) == 0 {
		return nil, nil
	}

	// Build an ID → bead map for fast lookup.
	idSet := make(map[string]*bead.Bead, len(beads))
	for _, b := range beads {
		idSet[b.ID] = b
	}

	// Compute in-degree (only considering in-slice deps) and reverse-edges.
	inDegree  := make(map[string]int, len(beads))
	dependents := make(map[string][]string, len(beads)) // depID → IDs of beads that depend on it

	for _, b := range beads {
		if _, ok := inDegree[b.ID]; !ok {
			inDegree[b.ID] = 0
		}
		for _, depID := range b.DependsOn {
			if _, ok := idSet[depID]; !ok {
				continue // external dep — ignore
			}
			inDegree[b.ID]++
			dependents[depID] = append(dependents[depID], b.ID)
		}
	}

	// Seed queue with zero-in-degree beads.
	var queue []string
	for _, b := range beads {
		if inDegree[b.ID] == 0 {
			queue = append(queue, b.ID)
		}
	}

	result := make([]*bead.Bead, 0, len(beads))
	processed := 0

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		result = append(result, idSet[id])
		processed++

		for _, nextID := range dependents[id] {
			inDegree[nextID]--
			if inDegree[nextID] == 0 {
				queue = append(queue, nextID)
			}
		}
	}

	if processed < len(beads) {
		return nil, fmt.Errorf("topoSortBeads: %w", ErrCycle)
	}
	return result, nil
}

// ─── ClaimAndExecute ──────────────────────────────────────────────────────────

// ClaimAndExecute peeks at the next sling:ready bead and routes to ExecuteEpic
// (epic path) or Execute (standalone path).
func ClaimAndExecute(opts ExecuteOptions) (*ClaimAndExecuteResult, error) {
	candidate, err := execClaimNextReady(opts.EpicID)
	if err != nil {
		return nil, err
	}
	if candidate == nil {
		fmt.Println("No sling:ready beads with all dependencies closed.")
		return &ClaimAndExecuteResult{Succeeded: false}, nil
	}

	if candidate.ParentID != "" {
		// Sub-bead → route to epic executor.
		epicOpts := EpicExecuteOptions{
			EpicID:          candidate.ParentID,
			RepoRoot:        opts.RepoRoot,
			ReviewMaxRounds: opts.ReviewMaxRounds,
			SpecMaxTurns:    opts.SpecMaxTurns,
			Notifier:        opts.Notifier,
			ContextFiles:    opts.ContextFiles,
		}
		epicResult, epicErr := execRouteEpic(epicOpts)
		if epicErr != nil {
			return &ClaimAndExecuteResult{IsEpic: true, EpicID: candidate.ParentID, Succeeded: false}, epicErr
		}
		return &ClaimAndExecuteResult{IsEpic: true, EpicID: candidate.ParentID, Succeeded: epicResult.Succeeded}, nil
	}

	// Standalone bead.
	execResult, execErr := execRouteStandalone(opts)
	if execErr != nil {
		beadID := ""
		if execResult != nil {
			beadID = execResult.BeadID
		}
		return &ClaimAndExecuteResult{IsEpic: false, BeadID: beadID, Succeeded: false}, execErr
	}
	beadID := ""
	if execResult != nil {
		beadID = execResult.BeadID
	}
	return &ClaimAndExecuteResult{IsEpic: false, BeadID: beadID, Succeeded: execResult != nil && execResult.Succeeded}, nil
}

// ─── ExecuteEpic ──────────────────────────────────────────────────────────────

// ExecuteEpic executes all ready sub-beads of an epic in a single shared worktree.
func ExecuteEpic(opts EpicExecuteOptions) (*EpicExecuteResult, error) {
	if opts.EpicID == "" {
		return nil, fmt.Errorf("ExecuteEpic: EpicID is required")
	}

	epicBead, err := execBeadShow(opts.EpicID)
	if err != nil {
		return nil, fmt.Errorf("ExecuteEpic: fetch epic bead: %w", err)
	}
	if epicBead.ParentID != "" {
		return nil, fmt.Errorf("ExecuteEpic: bead %s is a sub-bead (ParentID=%s), not an epic", opts.EpicID, epicBead.ParentID)
	}

	subBeads, err := execListSubBeads(opts.EpicID)
	if err != nil {
		return nil, fmt.Errorf("ExecuteEpic: list sub-beads: %w", err)
	}

	// Filter for ready sub-beads with all dependencies closed.
	var readySubBeads []*bead.Bead
	for _, b := range subBeads {
		if !bead.HasLabel(b, bead.LabelReady) {
			continue
		}
		allClosed, err := allDepsClosed(b.DependsOn)
		if err != nil {
			return nil, fmt.Errorf("ExecuteEpic: check deps for %s: %w", b.ID, err)
		}
		if allClosed {
			readySubBeads = append(readySubBeads, b)
		}
	}

	if len(readySubBeads) == 0 {
		fmt.Printf("No sub-beads of epic %q are ready.\n", opts.EpicID)
		return &EpicExecuteResult{EpicID: opts.EpicID, Succeeded: false}, nil
	}

	// Mark epic as executing.
	if err := execBeadSwapLabel(opts.EpicID, bead.LabelExecuting); err != nil {
		fmt.Printf("Warning: could not mark epic as executing: %v\n", err)
	}

	// Claim each ready sub-bead.
	var claimedIDs []string
	var claimedBeads []*bead.Bead
	for _, b := range readySubBeads {
		if err := execBeadClaimAndLabel(b.ID, bead.LabelExecuting, bead.LabelReady); err != nil {
			fmt.Printf("Warning: could not claim sub-bead %s: %v\n", b.ID, err)
			continue
		}
		claimedIDs = append(claimedIDs, b.ID)
		claimedBeads = append(claimedBeads, b)
	}

	// Guard: if every claim failed (e.g., racing worker claimed all beads),
	// abort rather than running an agent over zero sub-beads.
	if len(claimedIDs) == 0 {
		fmt.Printf("Warning: no sub-beads of epic %q could be claimed; skipping execution.\n", opts.EpicID)
		if swapErr := execBeadSwapLabel(opts.EpicID, bead.LabelReady); swapErr != nil {
			fmt.Printf("Warning: could not revert epic label: %v\n", swapErr)
		}
		return &EpicExecuteResult{EpicID: opts.EpicID, Succeeded: false}, nil
	}

	// Create shared worktree keyed by epicID.
	wt, err := execWorktreeAdd(opts.RepoRoot, opts.EpicID)
	if err != nil {
		// Failed to create worktree — mark everything failed.
		if swapErr := execBeadSwapLabel(opts.EpicID, bead.LabelFailed); swapErr != nil {
			fmt.Printf("Warning: could not mark epic as failed: %v\n", swapErr)
		}
		for _, id := range claimedIDs {
			if swapErr := execBeadSwapLabel(id, bead.LabelFailed); swapErr != nil {
				fmt.Printf("Warning: could not mark sub-bead %s as failed: %v\n", id, swapErr)
			}
		}
		return nil, fmt.Errorf("ExecuteEpic: create worktree: %w", err)
	}
	fmt.Printf("Epic worktree: %s\n", wt.Path)

	// Record worktree on epic bead (best-effort).
	if err := execBeadUpdateWorktree(opts.EpicID, wt.Path); err != nil {
		fmt.Printf("Warning: could not record worktree path on epic bead: %v\n", err)
	}

	// Topological sort of claimed sub-beads (best-effort; fall back to original order on error).
	sorted, topoErr := topoSortBeads(claimedBeads)
	if topoErr != nil {
		fmt.Printf("Warning: topological sort failed (%v); using original order\n", topoErr)
		sorted = claimedBeads
	}

	// Run SpecAgent for each sub-bead (best-effort); collect specs.
	specsByID := make(map[string]string, len(sorted))
	for _, b := range sorted {
		spec, specErr := execRunSpecAgent(b.ID, opts.RepoRoot, opts.ContextFiles, opts.SpecMaxTurns)
		if specErr != nil {
			fmt.Printf("Warning: spec agent failed for sub-bead %s (proceeding without spec): %v\n", b.ID, specErr)
		} else if spec != "" {
			specsByID[b.ID] = spec
		}
	}

	// Build EpicSubBeadSpec slice.
	subBeadSpecs := make([]agent.EpicSubBeadSpec, 0, len(sorted))
	for _, b := range sorted {
		subBeadSpecs = append(subBeadSpecs, agent.EpicSubBeadSpec{
			ID:    b.ID,
			Title: b.Title,
			Body:  b.Body,
			Spec:  specsByID[b.ID],
		})
	}

	// Build system prompt.
	prompt := agent.EpicExecutorSystemPrompt(opts.EpicID, epicBead.Title, epicBead.Body, subBeadSpecs, opts.ContextFiles)

	// Run executor agent.
	agentErr := execAgentRun(agent.RunOptions{
		WorkDir:      wt.Path,
		SystemPrompt: prompt,
		UserPrompt:   fmt.Sprintf("Implement the epic: %s", epicBead.Title),
		Model:        agent.ModelSonnet,
		MaxTurns:     100,
		Env:          map[string]string{"SLING_EPIC_ID": opts.EpicID},
	})

	if agentErr != nil {
		// Agent failed — mark everything failed and notify.
		if swapErr := execBeadSwapLabel(opts.EpicID, bead.LabelFailed); swapErr != nil {
			fmt.Printf("Warning: could not mark epic as failed: %v\n", swapErr)
		}
		for _, id := range claimedIDs {
			if swapErr := execBeadSwapLabel(id, bead.LabelFailed); swapErr != nil {
				fmt.Printf("Warning: could not mark sub-bead %s as failed: %v\n", id, swapErr)
			}
		}
		if opts.Notifier != nil {
			_ = opts.Notifier.Send(fmt.Sprintf("Sling: epic %q FAILED. ID: %s", epicBead.Title, opts.EpicID))
		}
		return &EpicExecuteResult{EpicID: opts.EpicID, WtPath: wt.Path, BeadIDs: claimedIDs, Succeeded: false}, agentErr
	}

	// Agent succeeded — transition sub-beads and epic to review-pending.
	for _, id := range claimedIDs {
		if swapErr := execBeadSwapLabel(id, bead.LabelReviewPending); swapErr != nil {
			fmt.Printf("Warning: could not mark sub-bead %s as review-pending: %v\n", id, swapErr)
		}
	}
	if swapErr := execBeadSwapLabel(opts.EpicID, bead.LabelReviewPending); swapErr != nil {
		fmt.Printf("Warning: could not mark epic as review-pending: %v\n", swapErr)
	}

	// Run automated review (best-effort).
	if reviewErr := execRunAutoReview(opts.EpicID, wt.Path, AutoReviewOptions{
		MaxRounds:    opts.ReviewMaxRounds,
		Notifier:     opts.Notifier,
		ContextFiles: opts.ContextFiles,
	}); reviewErr != nil {
		fmt.Printf("Warning: automated review error: %v\n", reviewErr)
	}

	return &EpicExecuteResult{EpicID: opts.EpicID, WtPath: wt.Path, BeadIDs: claimedIDs, Succeeded: true}, nil
}

// ─── SignalDoneEpic ───────────────────────────────────────────────────────────

// SignalDoneEpic is a manual rescue command that transitions an epic bead and
// all its sling:executing sub-beads to sling:review-pending.
func SignalDoneEpic(epicID string) error {
	epicBead, err := execBeadShow(epicID)
	if err != nil {
		return fmt.Errorf("signal-done-epic: fetch bead %s: %w", epicID, err)
	}
	if epicBead.ParentID != "" {
		return fmt.Errorf("bead %s is a sub-bead of epic %s — run: sling signal-done %s", epicID, epicBead.ParentID, epicBead.ParentID)
	}

	subBeads, err := execListSubBeads(epicID)
	if err != nil {
		return fmt.Errorf("signal-done-epic: list sub-beads: %w", err)
	}

	// Transition epic to review-pending.
	if swapErr := execBeadSwapLabel(epicID, bead.LabelReviewPending); swapErr != nil {
		fmt.Printf("Warning: could not mark epic %s as review-pending: %v\n", epicID, swapErr)
	}

	// Transition all executing sub-beads to review-pending.
	count := 0
	for _, b := range subBeads {
		if !bead.HasLabel(b, bead.LabelExecuting) {
			continue
		}
		if swapErr := execBeadSwapLabel(b.ID, bead.LabelReviewPending); swapErr != nil {
			fmt.Printf("Warning: could not mark sub-bead %s as review-pending: %v\n", b.ID, swapErr)
			continue
		}
		count++
	}

	fmt.Printf("✓ Epic %s and %d sub-beads marked review-pending\n", epicID, count)
	return nil
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
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			for _, marker := range markers {
				if strings.HasPrefix(trimmed, marker) {
					return fmt.Errorf("found") // abuse error to short-circuit
				}
			}
		}
		return nil
	})

	if err != nil && err.Error() == "found" {
		return true, nil
	}
	return false, err
}
