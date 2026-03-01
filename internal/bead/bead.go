// Package bead wraps the bd CLI for bead CRUD operations.
// bd is expected to be on PATH. All commands that return data use --json.
package bead

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Bead is the JSON-decoded representation of a bd bead.
type Bead struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Body        string   `json:"body"`
	Status      string   `json:"status"` // open, in_progress, blocked, closed
	Labels      []string `json:"labels"`
	ParentID    string   `json:"parent_id"`
	DependsOn   []string `json:"depends_on"` // bead IDs this bead depends on
	WorktreePath string  `json:"worktree_path"`
}

// run executes a bd command and returns stdout.
func run(args ...string) ([]byte, error) {
	cmd := exec.Command("bd", args...)
	out, err := cmd.Output()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("bd %s: %w\nstderr: %s", strings.Join(args, " "), err, e.Stderr)
		}
		return nil, fmt.Errorf("bd %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// Create creates a new bead and returns its ID.
// title is required; body, parentID are optional (pass "" to omit).
func Create(title, body, parentID string, labels []string) (string, error) {
	args := []string{"create", "--json", "--title", title}
	if body != "" {
		args = append(args, "--body", body)
	}
	if parentID != "" {
		args = append(args, "--parent", parentID)
	}
	for _, l := range labels {
		args = append(args, "--label", l)
	}

	out, err := run(args...)
	if err != nil {
		return "", err
	}

	var b Bead
	if err := json.Unmarshal(out, &b); err != nil {
		return "", fmt.Errorf("bead.Create decode: %w (raw: %s)", err, out)
	}
	return b.ID, nil
}

// Show returns the bead with the given ID.
func Show(id string) (*Bead, error) {
	out, err := run("show", "--json", id)
	if err != nil {
		return nil, err
	}
	var b Bead
	if err := json.Unmarshal(out, &b); err != nil {
		return nil, fmt.Errorf("bead.Show decode: %w", err)
	}
	return &b, nil
}

// List returns beads matching the given label filter.
// Pass "" for label to list all beads.
func List(label string) ([]*Bead, error) {
	args := []string{"list", "--json"}
	if label != "" {
		args = append(args, "--label", label)
	}
	out, err := run(args...)
	if err != nil {
		return nil, err
	}
	var beads []*Bead
	if err := json.Unmarshal(out, &beads); err != nil {
		return nil, fmt.Errorf("bead.List decode: %w", err)
	}
	return beads, nil
}

// SetStatus sets bead status: open, in_progress, blocked, closed.
func SetStatus(id, status string) error {
	_, err := run("update", "--json", id, "--status", status)
	return err
}

// AddLabel adds a label to a bead.
func AddLabel(id, label string) error {
	_, err := run("label", "--json", "add", id, label)
	return err
}

// RemoveLabel removes a label from a bead.
func RemoveLabel(id, label string) error {
	_, err := run("label", "--json", "remove", id, label)
	return err
}

// SetLabels replaces all labels on a bead with the given set.
func SetLabels(id string, labels []string) error {
	args := []string{"update", "--json", id}
	for _, l := range labels {
		args = append(args, "--label", l)
	}
	_, err := run(args...)
	return err
}

// UpdateBody sets the body/description of a bead.
func UpdateBody(id, body string) error {
	_, err := run("update", "--json", id, "--body", body)
	return err
}

// UpdateWorktree records the worktree path on the bead.
func UpdateWorktree(id, path string) error {
	_, err := run("update", "--json", id, "--worktree", path)
	return err
}

// SetDependsOn records dependency bead IDs on a bead.
func SetDependsOn(id string, depIDs []string) error {
	args := []string{"update", "--json", id}
	for _, dep := range depIDs {
		args = append(args, "--depends-on", dep)
	}
	_, err := run(args...)
	return err
}

// IsClosed returns true if the bead's status is "closed".
func IsClosed(id string) (bool, error) {
	b, err := Show(id)
	if err != nil {
		return false, err
	}
	return b.Status == "closed", nil
}

// HasLabel returns true if the bead has the given label.
func HasLabel(b *Bead, label string) bool {
	for _, l := range b.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// SlingLabel constants.
const (
	LabelPlanned        = "sling:planned"
	LabelReady          = "sling:ready"
	LabelExecuting      = "sling:executing"
	LabelReviewPending  = "sling:review-pending"
	LabelAddressing     = "sling:addressing"
	LabelFailed         = "sling:failed"
	LabelBlocked        = "sling:blocked"
)

// StatusOpen       = "open"
// StatusInProgress = "in_progress"
// StatusBlocked    = "blocked"
// StatusClosed     = "closed"
const (
	StatusOpen       = "open"
	StatusInProgress = "in_progress"
	StatusBlocked    = "blocked"
	StatusClosed     = "closed"
)
