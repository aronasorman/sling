package pipeline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/aronasorman/sling/internal/bead"
	"github.com/aronasorman/sling/internal/worktree"
)

func TestHasReviewMarkers(t *testing.T) {
	dir := t.TempDir()

	// No markers — should return false.
	if err := os.WriteFile(filepath.Join(dir, "clean.go"), []byte("package main\n\n// normal comment\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := HasReviewMarkers(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected no markers, got markers")
	}

	// Go-style marker.
	if err := os.WriteFile(filepath.Join(dir, "with_marker.go"), []byte("package main\n\n// REVIEW: fix this\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = HasReviewMarkers(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected markers, got none")
	}
}

func TestHasReviewMarkersStyles(t *testing.T) {
	markers := []string{
		"# REVIEW: python style",
		"// REVIEW: go style",
		"-- REVIEW: sql style",
		"<!-- REVIEW: html style",
		"/* REVIEW: c style",
	}
	for _, marker := range markers {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "file.go"), []byte(marker), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := HasReviewMarkers(dir)
		if err != nil {
			t.Fatalf("HasReviewMarkers(%q): %v", marker, err)
		}
		if !got {
			t.Errorf("HasReviewMarkers: marker %q not detected", marker)
		}
	}
}

func TestHasReviewMarkersSkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()

	// This tests that a REVIEW marker inside .git dir should be skipped.
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "COMMIT_EDITMSG"), []byte("// REVIEW: inside git"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := HasReviewMarkers(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected no markers (hidden .git dir should be skipped)")
	}
}

// ── TC-E1: ListSubBeads returns correct sub-beads ─────────────────────────────

func TestListSubBeads_ReturnsSubBeads(t *testing.T) {
	orig := execBeadList
	t.Cleanup(func() { execBeadList = orig })

	beadA := &bead.Bead{ID: "beadA", ParentID: ""}
	beadB := &bead.Bead{ID: "beadB", ParentID: "beadA"}
	beadC := &bead.Bead{ID: "beadC", ParentID: "beadA"}
	beadD := &bead.Bead{ID: "beadD", ParentID: "other"}

	execBeadList = func(label string) ([]*bead.Bead, error) {
		return []*bead.Bead{beadA, beadB, beadC, beadD}, nil
	}

	result, err := ListSubBeads("beadA")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 sub-beads, got %d: %v", len(result), result)
	}
	ids := sortedIDs(result)
	if ids[0] != "beadB" || ids[1] != "beadC" {
		t.Errorf("expected [beadB, beadC], got %v", ids)
	}
}

// TC-E2: ListSubBeads returns empty slice for standalone.
func TestListSubBeads_EmptyForStandalone(t *testing.T) {
	orig := execBeadList
	t.Cleanup(func() { execBeadList = orig })

	execBeadList = func(label string) ([]*bead.Bead, error) {
		return []*bead.Bead{
			{ID: "b1", ParentID: "other"},
		}, nil
	}

	result, err := ListSubBeads("xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}
	if result == nil {
		t.Error("expected non-nil empty slice")
	}
}

// ── TC-E3: topoSortBeads respects dependencies ────────────────────────────────

func TestTopoSortBeads_RespectsDeps(t *testing.T) {
	beadA := &bead.Bead{ID: "beadA", DependsOn: []string{}}
	beadB := &bead.Bead{ID: "beadB", DependsOn: []string{"beadA"}}
	beadC := &bead.Bead{ID: "beadC", DependsOn: []string{"beadA"}}
	beadD := &bead.Bead{ID: "beadD", DependsOn: []string{"beadB", "beadC"}}

	// Pass in reverse dependency order.
	input := []*bead.Bead{beadD, beadC, beadB, beadA}
	result, err := topoSortBeads(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 4 {
		t.Fatalf("expected 4 beads, got %d", len(result))
	}

	pos := make(map[string]int, 4)
	for i, b := range result {
		pos[b.ID] = i
	}

	if pos["beadA"] >= pos["beadB"] {
		t.Errorf("beadA must appear before beadB")
	}
	if pos["beadA"] >= pos["beadC"] {
		t.Errorf("beadA must appear before beadC")
	}
	if pos["beadB"] >= pos["beadD"] {
		t.Errorf("beadB must appear before beadD")
	}
	if pos["beadC"] >= pos["beadD"] {
		t.Errorf("beadC must appear before beadD")
	}
}

// TC-E4: topoSortBeads detects cycle.
func TestTopoSortBeads_DetectsCycle(t *testing.T) {
	beadX := &bead.Bead{ID: "beadX", DependsOn: []string{"beadY"}}
	beadY := &bead.Bead{ID: "beadY", DependsOn: []string{"beadX"}}

	_, err := topoSortBeads([]*bead.Bead{beadX, beadY})
	if err == nil {
		t.Fatal("expected error for cycle, got nil")
	}
	if !errors.Is(err, ErrCycle) {
		t.Errorf("expected ErrCycle, got %v", err)
	}
}

// ── TC-E5: ClaimAndExecute routes to ExecuteEpic for sub-bead ─────────────────

func TestClaimAndExecute_RoutesToEpic(t *testing.T) {
	origClaim := execClaimNextReady
	origEpic := execRouteEpic
	origStandalone := execRouteStandalone
	t.Cleanup(func() {
		execClaimNextReady = origClaim
		execRouteEpic = origEpic
		execRouteStandalone = origStandalone
	})

	subBead := &bead.Bead{ID: "s1", ParentID: "epicA", Labels: []string{bead.LabelReady}}
	execClaimNextReady = func() (*bead.Bead, error) { return subBead, nil }

	var calledEpicID string
	execRouteEpic = func(opts EpicExecuteOptions) (*EpicExecuteResult, error) {
		calledEpicID = opts.EpicID
		return &EpicExecuteResult{EpicID: opts.EpicID, Succeeded: true}, nil
	}

	var standaloneCalled bool
	execRouteStandalone = func(opts ExecuteOptions) (*ExecuteResult, error) {
		standaloneCalled = true
		return &ExecuteResult{Succeeded: true}, nil
	}

	result, err := ClaimAndExecute(ExecuteOptions{RepoRoot: "/tmp/repo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsEpic {
		t.Error("expected IsEpic == true")
	}
	if result.EpicID != "epicA" {
		t.Errorf("expected EpicID='epicA', got %q", result.EpicID)
	}
	if !result.Succeeded {
		t.Error("expected Succeeded == true")
	}
	if calledEpicID != "epicA" {
		t.Errorf("ExecuteEpic not called with EpicID='epicA'; got %q", calledEpicID)
	}
	if standaloneCalled {
		t.Error("Execute should NOT have been called for sub-bead")
	}
}

// TC-E6: ClaimAndExecute routes to Execute for standalone bead.
func TestClaimAndExecute_RoutesToStandalone(t *testing.T) {
	origClaim := execClaimNextReady
	origEpic := execRouteEpic
	origStandalone := execRouteStandalone
	t.Cleanup(func() {
		execClaimNextReady = origClaim
		execRouteEpic = origEpic
		execRouteStandalone = origStandalone
	})

	standaloneBead := &bead.Bead{ID: "b1", ParentID: "", Labels: []string{bead.LabelReady}}
	execClaimNextReady = func() (*bead.Bead, error) { return standaloneBead, nil }

	var epicCalled bool
	execRouteEpic = func(opts EpicExecuteOptions) (*EpicExecuteResult, error) {
		epicCalled = true
		return &EpicExecuteResult{Succeeded: true}, nil
	}

	var standaloneCalled bool
	execRouteStandalone = func(opts ExecuteOptions) (*ExecuteResult, error) {
		standaloneCalled = true
		return &ExecuteResult{BeadID: "b1", Succeeded: true}, nil
	}

	result, err := ClaimAndExecute(ExecuteOptions{RepoRoot: "/tmp/repo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsEpic {
		t.Error("expected IsEpic == false for standalone bead")
	}
	if !result.Succeeded {
		t.Error("expected Succeeded == true")
	}
	if !standaloneCalled {
		t.Error("expected Execute to be called for standalone bead")
	}
	if epicCalled {
		t.Error("ExecuteEpic should NOT have been called for standalone bead")
	}
}

// TC-E7: ClaimAndExecute returns Succeeded:false when no ready beads.
func TestClaimAndExecute_NoReadyBeads(t *testing.T) {
	orig := execClaimNextReady
	t.Cleanup(func() { execClaimNextReady = orig })

	execClaimNextReady = func() (*bead.Bead, error) { return nil, nil }

	result, err := ClaimAndExecute(ExecuteOptions{RepoRoot: "/tmp/repo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Succeeded {
		t.Error("expected Succeeded == false when no ready beads")
	}
}

// ── TC-E8: SignalDoneEpic transitions epic + executing sub-beads ──────────────

func TestSignalDoneEpic_TransitionsExecutingSubBeads(t *testing.T) {
	origShow := execBeadShow
	origList := execListSubBeads
	origSwap := execBeadSwapLabel
	t.Cleanup(func() {
		execBeadShow = origShow
		execListSubBeads = origList
		execBeadSwapLabel = origSwap
	})

	epicBead := &bead.Bead{ID: "e1", ParentID: ""}
	sub1 := &bead.Bead{ID: "s1", ParentID: "e1", Labels: []string{bead.LabelExecuting}}
	sub2 := &bead.Bead{ID: "s2", ParentID: "e1", Labels: []string{bead.LabelReviewPending}}
	sub3 := &bead.Bead{ID: "s3", ParentID: "e1", Labels: []string{bead.LabelFailed}}

	execBeadShow = func(id string) (*bead.Bead, error) {
		if id == "e1" {
			return epicBead, nil
		}
		return nil, fmt.Errorf("unknown bead %s", id)
	}
	execListSubBeads = func(epicID string) ([]*bead.Bead, error) {
		if epicID == "e1" {
			return []*bead.Bead{sub1, sub2, sub3}, nil
		}
		return nil, nil
	}

	swapped := make(map[string]string)
	execBeadSwapLabel = func(id, label string) error {
		swapped[id] = label
		return nil
	}

	if err := SignalDoneEpic("e1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Epic must be transitioned.
	if swapped["e1"] != bead.LabelReviewPending {
		t.Errorf("expected epic 'e1' to get LabelReviewPending, got %q", swapped["e1"])
	}
	// sub1 (executing) must be transitioned.
	if swapped["s1"] != bead.LabelReviewPending {
		t.Errorf("expected sub1 's1' to get LabelReviewPending, got %q", swapped["s1"])
	}
	// sub2 (review-pending) must NOT get an additional swap.
	if _, ok := swapped["s2"]; ok {
		t.Errorf("sub2 's2' (already review-pending) should NOT be swapped, but got %q", swapped["s2"])
	}
	// sub3 (failed) must NOT be swapped.
	if _, ok := swapped["s3"]; ok {
		t.Errorf("sub3 's3' (failed) should NOT be swapped, but got %q", swapped["s3"])
	}
}

// TC-E9: SignalDoneEpic rejects sub-bead IDs.
func TestSignalDoneEpic_RejectsSubBead(t *testing.T) {
	orig := execBeadShow
	t.Cleanup(func() { execBeadShow = orig })

	subBead := &bead.Bead{ID: "s1", ParentID: "e1"}
	execBeadShow = func(id string) (*bead.Bead, error) {
		if id == "s1" {
			return subBead, nil
		}
		return nil, fmt.Errorf("unknown")
	}

	err := SignalDoneEpic("s1")
	if err == nil {
		t.Fatal("expected error for sub-bead, got nil")
	}
	// Error should mention the redirect.
	errStr := err.Error()
	if !containsAny(errStr, "sub-bead", "ParentID") {
		t.Errorf("error message should mention sub-bead or ParentID, got: %s", errStr)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// sortedIDs returns sorted IDs from a bead slice.
func sortedIDs(beads []*bead.Bead) []string {
	ids := make([]string, len(beads))
	for i, b := range beads {
		ids[i] = b.ID
	}
	sort.Strings(ids)
	return ids
}

// containsAny returns true if s contains any of the given substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// unused import suppression — worktree is imported for type assertions in stubs
var _ = worktree.WorktreePath
