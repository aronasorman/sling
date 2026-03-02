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
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	Status    string            `json:"status"` // open, in_progress, blocked, closed
	Labels    []string          `json:"labels"`
	ParentID  string            `json:"parent_id"`
	DependsOn []string          `json:"depends_on"` // bead IDs this bead depends on
	Metadata  map[string]string `json:"metadata"`
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
	return createWithRig("", title, body, parentID, labels)
}

// CreateInRig creates a new bead in a specific gastown rig and returns its ID.
// rig is the rig name (e.g. "sling"); title is required; body, parentID are optional.
func CreateInRig(rig, title, body, parentID string, labels []string) (string, error) {
	return createWithRig(rig, title, body, parentID, labels)
}

// createWithRig is the shared implementation for Create and CreateInRig.
func createWithRig(rig, title, body, parentID string, labels []string) (string, error) {
	args := []string{"create", "--json", "--title", title}
	if body != "" {
		args = append(args, "--body", body)
	}
	if parentID != "" {
		args = append(args, "--parent", parentID)
	}
	if rig != "" {
		args = append(args, "--rig", rig)
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
// DependsOn is populated from metadata if not natively returned by bd.
func Show(id string) (*Bead, error) {
	out, err := run("show", "--json", id)
	if err != nil {
		return nil, err
	}
	// bd show --json returns an array; take the first element.
	var bs []Bead
	if err := json.Unmarshal(out, &bs); err != nil {
		// Fallback: try unmarshaling as a single object.
		var b0 Bead
		if err2 := json.Unmarshal(out, &b0); err2 != nil {
			return nil, fmt.Errorf("bead.Show decode: %w", err)
		}
		bs = []Bead{b0}
	}
	if len(bs) == 0 {
		return nil, fmt.Errorf("bead.Show: no bead found with id %s", id)
	}
	b := bs[0]
	// Populate DependsOn from metadata if bd did not return it natively.
	if len(b.DependsOn) == 0 && b.Metadata != nil {
		if deps, ok := b.Metadata["depends_on"]; ok && deps != "" {
			b.DependsOn = strings.Split(deps, ",")
		}
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
	_, err := run("update", "--json", id, "--add-label", label)
	return err
}

// RemoveLabel removes a label from a bead.
func RemoveLabel(id, label string) error {
	_, err := run("update", "--json", id, "--remove-label", label)
	return err
}

// Claim atomically claims a bead: sets status to in_progress and assignee to
// the current user. Fails with an error if the bead is already claimed.
// Use this instead of separate RemoveLabel/AddLabel/SetStatus calls to avoid races.
func Claim(id string) error {
	_, err := run("update", "--json", id, "--claim")
	return err
}

// SetLabels replaces all labels on a bead with the given set.
// Each label is passed as a separate --set-labels flag (the flag is repeatable,
// not comma-joined).
func SetLabels(id string, labels []string) error {
	args := []string{"update", "--json", id}
	for _, l := range labels {
		args = append(args, "--set-labels", l)
	}
	_, err := run(args...)
	return err
}

// SwapSlingLabel fetches the bead's current labels, removes every sling:* label,
// and adds newLabel — all in a single bd update call.
// Non-sling labels are left untouched.
func SwapSlingLabel(id, newLabel string) error {
	b, err := Show(id)
	if err != nil {
		return fmt.Errorf("bead.SwapSlingLabel: fetch labels: %w", err)
	}

	args := []string{"update", "--json", id}
	for _, l := range b.Labels {
		if strings.HasPrefix(l, "sling:") {
			args = append(args, "--remove-label", l)
		}
	}
	args = append(args, "--add-label", newLabel)
	_, err = run(args...)
	return err
}

// ClaimAndLabel atomically claims a bead (--claim) and swaps the sling label
// from removeLabel to addLabel in a single bd update call.
// Use this instead of separate Claim + SwapSlingLabel calls to avoid a window
// where the bead is claimed but still carries the old label.
func ClaimAndLabel(id, addLabel, removeLabel string) error {
	args := []string{"update", "--json", id, "--claim", "--add-label", addLabel, "--remove-label", removeLabel}
	_, err := run(args...)
	return err
}

// UpdateBody sets the body/description of a bead.
func UpdateBody(id, body string) error {
	_, err := run("update", "--json", id, "--body", body)
	return err
}

// UpdateWorktree records the worktree path in bead metadata.
func UpdateWorktree(id, path string) error {
	_, err := run("update", "--json", id, "--set-metadata", "worktree_path="+path)
	return err
}

// worktreeMarkerStart/End delimit the worktree path stored in the bead body
// (legacy fallback for beads written before the metadata approach).
const worktreeMarkerStart = "\n\n<!-- sling-worktree-path: "
const worktreeMarkerEnd = " -->"

// WorktreePathFromBead returns the worktree path for a bead.
// Checks metadata first, then falls back to the legacy body comment.
func WorktreePathFromBead(b *Bead) string {
	if b.Metadata != nil {
		if path, ok := b.Metadata["worktree_path"]; ok && path != "" {
			return path
		}
	}
	// Legacy fallback: parse the body comment written by older UpdateWorktree.
	body := b.Body
	start := strings.Index(body, worktreeMarkerStart)
	if start < 0 {
		return ""
	}
	rest := body[start+len(worktreeMarkerStart):]
	end := strings.Index(rest, worktreeMarkerEnd)
	if end < 0 {
		return "" // marker start found but end marker missing — return nothing
	}
	return rest[:end]
}

// SetDependsOn records dependency bead IDs in bead metadata.
// bd update does not expose a --deps flag; we use --set-metadata instead.
// Show() reads them back from metadata into DependsOn automatically.
func SetDependsOn(id string, depIDs []string) error {
	_, err := run("update", "--json", id, "--set-metadata", "depends_on="+strings.Join(depIDs, ","))
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
	LabelPlanned       = "sling:planned"
	LabelReady         = "sling:ready"
	LabelExecuting     = "sling:executing"
	LabelReviewPending = "sling:review-pending"
	LabelAddressing    = "sling:addressing"
	LabelFailed        = "sling:failed"
	LabelBlocked       = "sling:blocked"
	LabelEscalated     = "sling:escalated"
)

const (
	StatusOpen       = "open"
	StatusInProgress = "in_progress"
	StatusBlocked    = "blocked"
	StatusClosed     = "closed"
)
