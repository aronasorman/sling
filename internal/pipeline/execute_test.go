package pipeline

import (
	"os"
	"path/filepath"
	"testing"
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
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(marker), 0o644); err != nil {
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

	// REVIEW: marker inside .git — should be skipped.
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
