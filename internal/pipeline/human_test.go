package pipeline

import (
	"strings"
	"testing"

	"github.com/aronasorman/sling/internal/bead"
)

// ── TC-H1: Done redirects sub-bead to epic ────────────────────────────────────

func TestDone_RedirectsSubBead(t *testing.T) {
	origShow := humanBeadShow
	t.Cleanup(func() { humanBeadShow = origShow })

	humanBeadShow = func(id string) (*bead.Bead, error) {
		return &bead.Bead{ID: "s1", ParentID: "e1"}, nil
	}

	err := Done("s1", "/tmp/repo")
	if err == nil {
		t.Fatal("expected error for sub-bead redirect, got nil")
	}
	if !strings.Contains(err.Error(), "sling done e1") {
		t.Errorf("expected error to contain 'sling done e1', got: %s", err.Error())
	}
}

// ── TC-H2: Done routes epic to DoneEpic ──────────────────────────────────────

func TestDone_RoutesEpicToDoneEpic(t *testing.T) {
	origShow := humanBeadShow
	origList := humanListSubBeads
	origDoneEpic := humanDoneEpicFn
	t.Cleanup(func() {
		humanBeadShow = origShow
		humanListSubBeads = origList
		humanDoneEpicFn = origDoneEpic
	})

	humanBeadShow = func(id string) (*bead.Bead, error) {
		return &bead.Bead{ID: "e1", ParentID: ""}, nil
	}
	humanListSubBeads = func(epicID string) ([]*bead.Bead, error) {
		return []*bead.Bead{
			{ID: "s1", ParentID: "e1", Labels: []string{bead.LabelReviewPending}},
		}, nil
	}

	var doneEpicCalled bool
	var doneEpicID string
	humanDoneEpicFn = func(epicID, repoRoot string) error {
		doneEpicCalled = true
		doneEpicID = epicID
		return nil
	}

	err := Done("e1", "/tmp/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !doneEpicCalled {
		t.Error("expected DoneEpic to be called for epic bead")
	}
	if doneEpicID != "e1" {
		t.Errorf("expected DoneEpic called with 'e1', got %q", doneEpicID)
	}
}

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
