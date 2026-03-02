package pipeline

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aronasorman/sling/internal/bead"
)

// ── TC-H3: DoneEpic closes epic and all sub-beads ────────────────────────────

func TestDoneEpic_ClosesEpicAndSubBeads(t *testing.T) {
	origShow := humanBeadShow
	origList := humanListSubBeads
	origMarkers := humanHasReviewMarkers
	origSquash := humanWorktreeSquash
	origPush := humanWorktreePushBranch
	origRemove := humanWorktreeRemove
	origSetStatus := humanBeadSetStatus
	origRemoveLabel := humanBeadRemoveLabel
	origWtPath := humanWorktreePathFn
	t.Cleanup(func() {
		humanBeadShow = origShow
		humanListSubBeads = origList
		humanHasReviewMarkers = origMarkers
		humanWorktreeSquash = origSquash
		humanWorktreePushBranch = origPush
		humanWorktreeRemove = origRemove
		humanBeadSetStatus = origSetStatus
		humanBeadRemoveLabel = origRemoveLabel
		humanWorktreePathFn = origWtPath
	})

	epicBead := &bead.Bead{ID: "e1", Title: "My Epic", ParentID: ""}
	sub1 := &bead.Bead{ID: "s1", ParentID: "e1", Labels: []string{bead.LabelReviewPending}, Status: "open"}
	sub2 := &bead.Bead{ID: "s2", ParentID: "e1", Status: bead.StatusClosed}

	humanBeadShow = func(id string) (*bead.Bead, error) {
		if id == "e1" {
			return epicBead, nil
		}
		return nil, nil
	}
	humanListSubBeads = func(epicID string) ([]*bead.Bead, error) {
		return []*bead.Bead{sub1, sub2}, nil
	}
	humanHasReviewMarkers = func(dir string) (bool, error) { return false, nil }

	var squashMsg, squashPath string
	humanWorktreeSquash = func(wtPath, msg string) error {
		squashPath = wtPath
		squashMsg = msg
		return nil
	}

	var pushBranch string
	humanWorktreePushBranch = func(wtPath, branch, remote string) error {
		pushBranch = branch
		return nil
	}

	var removedID string
	humanWorktreeRemove = func(repoRoot, id string) error {
		removedID = id
		return nil
	}

	closedIDs := make(map[string]string)
	humanBeadSetStatus = func(id, status string) error {
		closedIDs[id] = status
		return nil
	}

	var removedLabels []string
	humanBeadRemoveLabel = func(id, label string) error {
		removedLabels = append(removedLabels, id+":"+label)
		return nil
	}

	humanWorktreePathFn = func(repoRoot, id string) string {
		return "/tmp/wt/" + id
	}

	err := DoneEpic("e1", "/tmp/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Squash called with message containing epic ID and title.
	if !strings.Contains(squashMsg, "e1") || !strings.Contains(squashMsg, "My Epic") {
		t.Errorf("squash message should contain 'e1' and 'My Epic', got %q", squashMsg)
	}

	// PushBranch called with branch "sling/e1".
	if pushBranch != "sling/e1" {
		t.Errorf("expected push branch 'sling/e1', got %q", pushBranch)
	}

	// sub1 should be closed (sub2 is already closed).
	if closedIDs["s1"] != bead.StatusClosed {
		t.Errorf("expected s1 status closed, got %q", closedIDs["s1"])
	}
	if _, ok := closedIDs["s2"]; ok {
		t.Errorf("sub2 is already closed — should not be set again")
	}

	// Epic should be closed.
	if closedIDs["e1"] != bead.StatusClosed {
		t.Errorf("expected e1 status closed, got %q", closedIDs["e1"])
	}

	// worktree.Remove called for epic ID.
	if removedID != "e1" {
		t.Errorf("expected worktree.Remove called with 'e1', got %q", removedID)
	}

	_ = squashPath
}

// TC-H4: DoneEpic rejects when REVIEW: markers present.
func TestDoneEpic_RejectsWhenReviewMarkersPresent(t *testing.T) {
	origShow := humanBeadShow
	origList := humanListSubBeads
	origMarkers := humanHasReviewMarkers
	origWtPath := humanWorktreePathFn
	t.Cleanup(func() {
		humanBeadShow = origShow
		humanListSubBeads = origList
		humanHasReviewMarkers = origMarkers
		humanWorktreePathFn = origWtPath
	})

	humanBeadShow = func(id string) (*bead.Bead, error) {
		return &bead.Bead{ID: "e1", Title: "Epic", ParentID: ""}, nil
	}
	humanListSubBeads = func(epicID string) ([]*bead.Bead, error) {
		return []*bead.Bead{
			{ID: "s1", ParentID: "e1", Labels: []string{bead.LabelReviewPending}, Status: "open"},
		}, nil
	}
	humanHasReviewMarkers = func(dir string) (bool, error) { return true, nil }
	humanWorktreePathFn = func(repoRoot, id string) string { return "/tmp/wt/" + id }

	err := DoneEpic("e1", "/tmp/repo")
	if err == nil {
		t.Fatal("expected error when REVIEW: markers present, got nil")
	}
	if !strings.Contains(err.Error(), "REVIEW: markers") {
		t.Errorf("expected error about REVIEW: markers, got: %s", err.Error())
	}
}

// TC-H5: DoneEpic rejects when sub-bead not ready.
func TestDoneEpic_RejectsSubBeadNotReady(t *testing.T) {
	origShow := humanBeadShow
	origList := humanListSubBeads
	t.Cleanup(func() {
		humanBeadShow = origShow
		humanListSubBeads = origList
	})

	humanBeadShow = func(id string) (*bead.Bead, error) {
		return &bead.Bead{ID: "e1", Title: "Epic", ParentID: ""}, nil
	}
	humanListSubBeads = func(epicID string) ([]*bead.Bead, error) {
		return []*bead.Bead{
			// sub-bead with sling:executing — not acceptable for Done
			{ID: "s1", ParentID: "e1", Labels: []string{bead.LabelExecuting}, Status: "in_progress"},
		}, nil
	}

	err := DoneEpic("e1", "/tmp/repo")
	if err == nil {
		t.Fatal("expected error for executing sub-bead, got nil")
	}
	if !strings.Contains(err.Error(), "s1") {
		t.Errorf("expected error to mention sub-bead 's1', got: %s", err.Error())
	}
}

// ── EpicProgress tests (TC-EP-1 through TC-EP-6) ─────────────────────────────

// TC-EP-1: epicID="" returns (0, 0, nil) without calling humanBeadList.
func TestEpicProgress_EmptyEpicID(t *testing.T) {
	orig := humanBeadList
	t.Cleanup(func() { humanBeadList = orig })

	var called bool
	humanBeadList = func(label string) ([]*bead.Bead, error) {
		called = true
		return nil, fmt.Errorf("should not be called")
	}

	closed, total, err := EpicProgress("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if closed != 0 || total != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", closed, total)
	}
	if called {
		t.Error("humanBeadList should not be called when epicID is empty")
	}
}

// TC-EP-2: epicID="ep1", 3 open beads → (0, 3, nil).
func TestEpicProgress_AllOpen(t *testing.T) {
	orig := humanBeadList
	t.Cleanup(func() { humanBeadList = orig })

	humanBeadList = func(label string) ([]*bead.Bead, error) {
		return []*bead.Bead{
			{ID: "b1", ParentID: "ep1", Status: "open"},
			{ID: "b2", ParentID: "ep1", Status: "in_progress"},
			{ID: "b3", ParentID: "ep1", Status: "blocked"},
		}, nil
	}

	closed, total, err := EpicProgress("ep1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if closed != 0 {
		t.Errorf("expected closed=0, got %d", closed)
	}
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}
}

// TC-EP-3: epicID="ep1", 2 closed, 1 open → (2, 3, nil).
func TestEpicProgress_PartialClosed(t *testing.T) {
	orig := humanBeadList
	t.Cleanup(func() { humanBeadList = orig })

	humanBeadList = func(label string) ([]*bead.Bead, error) {
		return []*bead.Bead{
			{ID: "b1", ParentID: "ep1", Status: bead.StatusClosed},
			{ID: "b2", ParentID: "ep1", Status: bead.StatusClosed},
			{ID: "b3", ParentID: "ep1", Status: "open"},
			{ID: "b4", ParentID: "other", Status: bead.StatusClosed}, // different epic, ignored
		}, nil
	}

	closed, total, err := EpicProgress("ep1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if closed != 2 {
		t.Errorf("expected closed=2, got %d", closed)
	}
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}
}

// TC-EP-4: epicID="ep1", 3 closed → (3, 3, nil).
func TestEpicProgress_AllClosed(t *testing.T) {
	orig := humanBeadList
	t.Cleanup(func() { humanBeadList = orig })

	humanBeadList = func(label string) ([]*bead.Bead, error) {
		return []*bead.Bead{
			{ID: "b1", ParentID: "ep1", Status: bead.StatusClosed},
			{ID: "b2", ParentID: "ep1", Status: bead.StatusClosed},
			{ID: "b3", ParentID: "ep1", Status: bead.StatusClosed},
		}, nil
	}

	closed, total, err := EpicProgress("ep1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if closed != 3 {
		t.Errorf("expected closed=3, got %d", closed)
	}
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}
}

// TC-EP-5: epicID="ep1", 0 children → (0, 0, nil).
func TestEpicProgress_NoChildren(t *testing.T) {
	orig := humanBeadList
	t.Cleanup(func() { humanBeadList = orig })

	humanBeadList = func(label string) ([]*bead.Bead, error) {
		return []*bead.Bead{
			{ID: "b1", ParentID: "other", Status: bead.StatusClosed},
		}, nil
	}

	closed, total, err := EpicProgress("ep1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if closed != 0 || total != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", closed, total)
	}
}

// TC-EP-6: humanBeadList returns error → propagates.
func TestEpicProgress_ListError(t *testing.T) {
	orig := humanBeadList
	t.Cleanup(func() { humanBeadList = orig })

	humanBeadList = func(label string) ([]*bead.Bead, error) {
		return nil, fmt.Errorf("bd failure")
	}

	_, _, err := EpicProgress("ep1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── maybeCloseEpic tests (TC-MCE-1 through TC-MCE-5) ─────────────────────────

// TC-MCE-1: epicID="" → no-op, returns nil.
func TestMaybeCloseEpic_EmptyEpicID(t *testing.T) {
	orig := humanBeadList
	t.Cleanup(func() { humanBeadList = orig })

	var called bool
	humanBeadList = func(label string) ([]*bead.Bead, error) {
		called = true
		return nil, nil
	}

	err := maybeCloseEpic("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("humanBeadList should not be called for empty epicID")
	}
}

// TC-MCE-2: 2/3 children closed → no-op (epic not closed).
func TestMaybeCloseEpic_PartialClosed(t *testing.T) {
	origList := humanBeadList
	origSetStatus := humanBeadSetStatus
	t.Cleanup(func() {
		humanBeadList = origList
		humanBeadSetStatus = origSetStatus
	})

	humanBeadList = func(label string) ([]*bead.Bead, error) {
		return []*bead.Bead{
			{ID: "b1", ParentID: "ep1", Status: bead.StatusClosed},
			{ID: "b2", ParentID: "ep1", Status: bead.StatusClosed},
			{ID: "b3", ParentID: "ep1", Status: "open"},
		}, nil
	}

	var setStatusCalled bool
	humanBeadSetStatus = func(id, status string) error {
		setStatusCalled = true
		return nil
	}

	err := maybeCloseEpic("ep1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if setStatusCalled {
		t.Error("SetStatus should not be called when epic is not fully closed")
	}
}

// TC-MCE-3: 0/0 children → no-op (total == 0).
func TestMaybeCloseEpic_NoChildren(t *testing.T) {
	origList := humanBeadList
	origSetStatus := humanBeadSetStatus
	t.Cleanup(func() {
		humanBeadList = origList
		humanBeadSetStatus = origSetStatus
	})

	humanBeadList = func(label string) ([]*bead.Bead, error) {
		return []*bead.Bead{}, nil
	}

	var setStatusCalled bool
	humanBeadSetStatus = func(id, status string) error {
		setStatusCalled = true
		return nil
	}

	err := maybeCloseEpic("ep1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if setStatusCalled {
		t.Error("SetStatus should not be called when there are no children")
	}
}

// TC-MCE-4: 3/3 children closed → SetStatus called, labels removed, completion printed.
func TestMaybeCloseEpic_AllClosed(t *testing.T) {
	origList := humanBeadList
	origSetStatus := humanBeadSetStatus
	origRemoveLabel := humanBeadRemoveLabel
	t.Cleanup(func() {
		humanBeadList = origList
		humanBeadSetStatus = origSetStatus
		humanBeadRemoveLabel = origRemoveLabel
	})

	humanBeadList = func(label string) ([]*bead.Bead, error) {
		return []*bead.Bead{
			{ID: "b1", ParentID: "ep1", Status: bead.StatusClosed},
			{ID: "b2", ParentID: "ep1", Status: bead.StatusClosed},
			{ID: "b3", ParentID: "ep1", Status: bead.StatusClosed},
		}, nil
	}

	var setStatusID, setStatusValue string
	humanBeadSetStatus = func(id, status string) error {
		setStatusID = id
		setStatusValue = status
		return nil
	}

	var removedLabels []string
	humanBeadRemoveLabel = func(id, label string) error {
		removedLabels = append(removedLabels, id+":"+label)
		return nil
	}

	err := maybeCloseEpic("ep1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if setStatusID != "ep1" || setStatusValue != bead.StatusClosed {
		t.Errorf("expected SetStatus(ep1, closed), got SetStatus(%s, %s)", setStatusID, setStatusValue)
	}

	// At least some labels should have been removed.
	if len(removedLabels) == 0 {
		t.Error("expected label removals for epic, got none")
	}
}

// TC-MCE-5: 3/3, SetStatus errors → returns the error.
func TestMaybeCloseEpic_SetStatusError(t *testing.T) {
	origList := humanBeadList
	origSetStatus := humanBeadSetStatus
	origRemoveLabel := humanBeadRemoveLabel
	t.Cleanup(func() {
		humanBeadList = origList
		humanBeadSetStatus = origSetStatus
		humanBeadRemoveLabel = origRemoveLabel
	})

	humanBeadList = func(label string) ([]*bead.Bead, error) {
		return []*bead.Bead{
			{ID: "b1", ParentID: "ep1", Status: bead.StatusClosed},
			{ID: "b2", ParentID: "ep1", Status: bead.StatusClosed},
			{ID: "b3", ParentID: "ep1", Status: bead.StatusClosed},
		}, nil
	}

	humanBeadSetStatus = func(id, status string) error {
		return fmt.Errorf("bd set-status failure")
	}

	var removeLabelCalled bool
	humanBeadRemoveLabel = func(id, label string) error {
		removeLabelCalled = true
		return nil
	}

	err := maybeCloseEpic("ep1")
	if err == nil {
		t.Fatal("expected error from SetStatus, got nil")
	}

	if removeLabelCalled {
		t.Error("RemoveLabel should not be called after SetStatus failure")
	}
}

// ── Done epic block tests (TC-DONE-1 through TC-DONE-4) ──────────────────────

// setupDoneStubs is a helper that stubs all injectable vars needed for Done
// and restores them in t.Cleanup. It returns a struct for caller to configure.
type doneStubs struct {
	beadsByID     map[string]*bead.Bead
	closedIDs     map[string]string   // id → status set
	removedLabels []string            // "id:label"
	squashedPath  string
	pushedBranch  string
	// epic beads for humanBeadList (all beads in system)
	allBeads []*bead.Bead
}

func setupDoneStubs(t *testing.T, s *doneStubs) {
	t.Helper()

	origShow := humanBeadShow
	origWtPathFromBead := humanWorktreePathFromBead
	origWtPathFn := humanWorktreePathFn
	origMarkers := humanHasReviewMarkers
	origSquash := humanWorktreeSquash
	origPush := humanWorktreePushBranch
	origSetStatus := humanBeadSetStatus
	origRemoveLabel := humanBeadRemoveLabel
	origBeadList := humanBeadList
	origListSubBeads := humanListSubBeads
	origDoneEpicFn := humanDoneEpicFn
	origExpandList := expandBeadList
	origExpandIsClosed := expandBeadIsClosed
	origExpandRemove := expandBeadRemoveLabel
	origExpandAdd := expandBeadAddLabel
	t.Cleanup(func() {
		humanBeadShow = origShow
		humanWorktreePathFromBead = origWtPathFromBead
		humanWorktreePathFn = origWtPathFn
		humanHasReviewMarkers = origMarkers
		humanWorktreeSquash = origSquash
		humanWorktreePushBranch = origPush
		humanBeadSetStatus = origSetStatus
		humanBeadRemoveLabel = origRemoveLabel
		humanBeadList = origBeadList
		humanListSubBeads = origListSubBeads
		humanDoneEpicFn = origDoneEpicFn
		expandBeadList = origExpandList
		expandBeadIsClosed = origExpandIsClosed
		expandBeadRemoveLabel = origExpandRemove
		expandBeadAddLabel = origExpandAdd
	})

	humanBeadShow = func(id string) (*bead.Bead, error) {
		if b, ok := s.beadsByID[id]; ok {
			return b, nil
		}
		return nil, fmt.Errorf("bead %s not found", id)
	}
	humanWorktreePathFromBead = func(b *bead.Bead) string { return "" }
	humanWorktreePathFn = func(repoRoot, id string) string { return "/tmp/wt/" + id }
	humanHasReviewMarkers = func(dir string) (bool, error) { return false, nil }
	humanWorktreeSquash = func(wtPath, msg string) error {
		s.squashedPath = wtPath
		return nil
	}
	humanWorktreePushBranch = func(wtPath, branch, remote string) error {
		s.pushedBranch = branch
		return nil
	}
	humanBeadSetStatus = func(id, status string) error {
		s.closedIDs[id] = status
		return nil
	}
	humanBeadRemoveLabel = func(id, label string) error {
		s.removedLabels = append(s.removedLabels, id+":"+label)
		return nil
	}
	humanBeadList = func(label string) ([]*bead.Bead, error) {
		return s.allBeads, nil
	}
	// humanListSubBeads: filter allBeads by ParentID.
	humanListSubBeads = func(epicID string) ([]*bead.Bead, error) {
		var result []*bead.Bead
		for _, b := range s.allBeads {
			if b.ParentID == epicID {
				result = append(result, b)
			}
		}
		return result, nil
	}
	// humanDoneEpicFn: default no-op stub (tests can override).
	humanDoneEpicFn = func(epicID, repoRoot string) error { return nil }
	// PromoteReadyBeads uses expand injectable vars — return empty to make it a no-op.
	expandBeadList = func(label string) ([]*bead.Bead, error) { return nil, nil }
	expandBeadIsClosed = func(id string) (bool, error) { return true, nil }
	expandBeadRemoveLabel = func(id, label string) error { return nil }
	expandBeadAddLabel = func(id, label string) error { return nil }
}

// TC-H1: Done redirects sub-bead to its epic.
func TestDone_SubBead_RedirectsToEpic(t *testing.T) {
	s := &doneStubs{
		beadsByID: map[string]*bead.Bead{
			"s1": {ID: "s1", Title: "Sub Bead", ParentID: "e1"},
		},
		closedIDs: make(map[string]string),
		allBeads:  []*bead.Bead{},
	}
	setupDoneStubs(t, s)

	err := Done("s1", "/tmp/repo")
	if err == nil {
		t.Fatal("expected error for sub-bead Done, got nil")
	}
	if !strings.Contains(err.Error(), "sling done e1") {
		t.Errorf("expected error to contain 'sling done e1', got: %s", err.Error())
	}
}

// TC-H2: Done routes epic bead (with sub-beads) to DoneEpic.
func TestDone_EpicBead_RoutesToDoneEpic(t *testing.T) {
	s := &doneStubs{
		beadsByID: map[string]*bead.Bead{
			"e1": {ID: "e1", Title: "Epic", ParentID: ""},
		},
		closedIDs: make(map[string]string),
		allBeads: []*bead.Bead{
			{ID: "s1", ParentID: "e1", Labels: []string{bead.LabelReviewPending}, Status: "open"},
		},
	}
	setupDoneStubs(t, s)

	var doneEpicCalled string
	humanDoneEpicFn = func(epicID, repoRoot string) error {
		doneEpicCalled = epicID
		return nil
	}

	err := Done("e1", "/tmp/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doneEpicCalled != "e1" {
		t.Errorf("expected DoneEpic called with 'e1', got %q", doneEpicCalled)
	}
}

// TC-DONE-3: beadID=B, ParentID="" (standalone)
// → PromoteReadyBeads NOT called; no epic progress.
func TestDone_Standalone_NoEpicLogic(t *testing.T) {
	s := &doneStubs{
		beadsByID: map[string]*bead.Bead{
			"B": {ID: "B", Title: "Standalone Bead", ParentID: ""},
		},
		closedIDs: make(map[string]string),
		allBeads:  []*bead.Bead{},
	}
	var expandListCalled bool
	setupDoneStubs(t, s)
	// Override to detect if PromoteReadyBeads tries to call list.
	expandBeadList = func(label string) ([]*bead.Bead, error) {
		expandListCalled = true
		return nil, nil
	}

	err := Done("B", "/tmp/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Bead B should be closed.
	if s.closedIDs["B"] != bead.StatusClosed {
		t.Errorf("expected B to be closed")
	}

	// Epic ep1 should NOT be involved.
	if _, ok := s.closedIDs["ep1"]; ok {
		t.Error("epic should not be closed for standalone bead")
	}

	// expandBeadList should NOT be called (PromoteReadyBeads skipped when ParentID=="").
	if expandListCalled {
		t.Error("PromoteReadyBeads should not be called for standalone bead (no ParentID)")
	}
}

// TC-DONE-4: REVIEW markers in worktree → returns error before squash.
func TestDone_ReviewMarkersPresent_ReturnsError(t *testing.T) {
	origShow := humanBeadShow
	origWtPathFromBead := humanWorktreePathFromBead
	origWtPathFn := humanWorktreePathFn
	origMarkers := humanHasReviewMarkers
	origSquash := humanWorktreeSquash
	origListSubBeads := humanListSubBeads
	t.Cleanup(func() {
		humanBeadShow = origShow
		humanWorktreePathFromBead = origWtPathFromBead
		humanWorktreePathFn = origWtPathFn
		humanHasReviewMarkers = origMarkers
		humanWorktreeSquash = origSquash
		humanListSubBeads = origListSubBeads
	})

	humanBeadShow = func(id string) (*bead.Bead, error) {
		return &bead.Bead{ID: "B", Title: "Bead B", ParentID: ""}, nil
	}
	humanWorktreePathFromBead = func(b *bead.Bead) string { return "" }
	humanWorktreePathFn = func(repoRoot, id string) string { return "/tmp/wt/" + id }
	humanHasReviewMarkers = func(dir string) (bool, error) { return true, nil }
	// Standalone bead has no sub-beads.
	humanListSubBeads = func(epicID string) ([]*bead.Bead, error) { return []*bead.Bead{}, nil }

	var squashCalled bool
	humanWorktreeSquash = func(wtPath, msg string) error {
		squashCalled = true
		return nil
	}

	err := Done("B", "/tmp/repo")
	if err == nil {
		t.Fatal("expected error when REVIEW: markers present, got nil")
	}
	if !strings.Contains(err.Error(), "REVIEW: markers") {
		t.Errorf("expected error about REVIEW: markers, got: %s", err.Error())
	}
	if squashCalled {
		t.Error("squash should not be called when REVIEW: markers present")
	}
}
