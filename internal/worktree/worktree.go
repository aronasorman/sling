// Package worktree wraps the jj CLI for workspace/worktree management.
package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Workspace represents a jj workspace (worktree).
type Workspace struct {
	Path   string
	Branch string
	BeadID string
}

// run executes a jj command in the given working directory.
func run(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("jj", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("jj %s: %w\nstderr: %s", strings.Join(args, " "), err, e.Stderr)
		}
		return nil, fmt.Errorf("jj %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// WorktreePath returns the path for a bead's worktree under the repo root.
func WorktreePath(repoRoot, beadID string) string {
	return filepath.Join(repoRoot, ".sling-worktrees", beadID)
}

// Add creates a new jj workspace at the given path with a new branch named after beadID.
// repoRoot is the main jj repo directory.
func Add(repoRoot, beadID string) (*Workspace, error) {
	wtPath := WorktreePath(repoRoot, beadID)
	branch := "sling/" + beadID

	if err := os.MkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating worktrees dir: %w", err)
	}

	// jj workspace add --name <beadID> <path>
	if _, err := run(repoRoot, "workspace", "add", "--name", beadID, wtPath); err != nil {
		return nil, fmt.Errorf("jj workspace add: %w", err)
	}

	// Create and set the branch in the new workspace.
	if _, err := run(wtPath, "branch", "create", branch); err != nil {
		return nil, fmt.Errorf("jj branch create: %w", err)
	}

	return &Workspace{
		Path:   wtPath,
		Branch: branch,
		BeadID: beadID,
	}, nil
}

// Remove removes the jj workspace for the given bead.
func Remove(repoRoot, beadID string) error {
	wtPath := WorktreePath(repoRoot, beadID)
	_, err := run(repoRoot, "workspace", "forget", beadID)
	if err != nil {
		return fmt.Errorf("jj workspace forget: %w", err)
	}
	return os.RemoveAll(wtPath)
}

// Squash squashes all commits in the worktree into one, using the given message.
func Squash(wtPath, message string) error {
	_, err := run(wtPath, "squash", "--message", message)
	return err
}

// PushBranch pushes the branch to the given remote (default "origin").
func PushBranch(wtPath, branch, remote string) error {
	if remote == "" {
		remote = "origin"
	}
	_, err := run(wtPath, "git", "push", "--remote", remote, "--branch", branch)
	return err
}

// NewCommit creates a new empty commit in the worktree (for review).
func NewCommit(wtPath, message string) error {
	_, err := run(wtPath, "new", "--message", message)
	return err
}

// CommitMessage returns the commit message of the current working-copy commit.
func CommitMessage(wtPath string) (string, error) {
	out, err := run(wtPath, "log", "--no-graph", "--revisions", "@", "--template", "description")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// DetectRepoRoot walks up from dir looking for a .jj directory.
// Falls back to dir itself if not found.
func DetectRepoRoot(dir string) string {
	d := dir
	for {
		if _, err := os.Stat(filepath.Join(d, ".jj")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return dir
}
