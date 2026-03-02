package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectGastownRig_Found(t *testing.T) {
	tmp := t.TempDir()

	// Create a gastown rig config.json in a parent directory.
	rigDir := filepath.Join(tmp, "myrig")
	if err := os.MkdirAll(rigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configJSON := `{"type":"rig","name":"myproject"}`
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Start from a subdirectory.
	subDir := filepath.Join(rigDir, "sub", "deep")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got := detectGastownRig(subDir)
	if got != "myproject" {
		t.Errorf("detectGastownRig(%q) = %q; want %q", subDir, got, "myproject")
	}
}

func TestDetectGastownRig_NotFound(t *testing.T) {
	tmp := t.TempDir()
	got := detectGastownRig(tmp)
	if got != "" {
		t.Errorf("detectGastownRig(%q) = %q; want empty string", tmp, got)
	}
}

func TestDetectGastownRig_WrongType(t *testing.T) {
	tmp := t.TempDir()
	// config.json exists but type != "rig".
	configJSON := `{"type":"town","name":"mytown"}`
	if err := os.WriteFile(filepath.Join(tmp, "config.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	got := detectGastownRig(tmp)
	if got != "" {
		t.Errorf("detectGastownRig with type=town: got %q; want empty string", got)
	}
}

func TestParseGitHubRepoFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/owner/repo.git", "owner/repo"},
		{"https://github.com/owner/repo", "owner/repo"},
		{"git@github.com:owner/repo.git", "owner/repo"},
		{"git@github.com:owner/repo", "owner/repo"},
		{"https://gitlab.com/owner/repo.git", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseGitHubRepoFromURL(tt.url)
		if got != tt.want {
			t.Errorf("parseGitHubRepoFromURL(%q) = %q; want %q", tt.url, got, tt.want)
		}
	}
}
