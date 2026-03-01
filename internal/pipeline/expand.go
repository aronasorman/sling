package pipeline

import (
	"fmt"

	"github.com/aronasorman/sling/internal/bead"
)

// ExpandResult maps plan bead index → bd bead ID.
type ExpandResult struct {
	EpicID      string
	BeadsByIndex map[int]string // plan index → bead ID
}

// Expand creates bd child beads from the plan and labels them sling:ready (or sling:planned
// if they have unmet dependencies). It resolves depends_on indices to bead IDs.
//
// Per issue #7: the index→ID mapping is stored in ExpandResult so the executor
// can later check dependency status.
func Expand(plan *Plan, epicID string) (*ExpandResult, error) {
	result := &ExpandResult{
		EpicID:      epicID,
		BeadsByIndex: make(map[int]string),
	}

	// First pass: create all beads as sling:planned.
	for _, pb := range plan.Beads {
		id, err := bead.Create(pb.Title, pb.Body, epicID, []string{bead.LabelPlanned})
		if err != nil {
			return nil, fmt.Errorf("expand: create bead %q: %w", pb.Title, err)
		}
		result.BeadsByIndex[pb.Index] = id
		fmt.Printf("  Created bead %d/%d: %s (id=%s)\n", pb.Index, len(plan.Beads), pb.Title, id)
	}

	// Second pass: set depends_on relationships and promote ready beads.
	for _, pb := range plan.Beads {
		beadID := result.BeadsByIndex[pb.Index]

		// Resolve depends_on indices to bead IDs.
		var depIDs []string
		for _, depIdx := range pb.DependsOn {
			depID, ok := result.BeadsByIndex[depIdx]
			if !ok {
				return nil, fmt.Errorf("expand: bead %d references unknown dependency index %d", pb.Index, depIdx)
			}
			depIDs = append(depIDs, depID)
		}

		if len(depIDs) > 0 {
			if err := bead.SetDependsOn(beadID, depIDs); err != nil {
				return nil, fmt.Errorf("expand: set depends_on for bead %s: %w", beadID, err)
			}
		}

		// If no dependencies, promote to sling:ready immediately.
		if len(pb.DependsOn) == 0 {
			if err := promoteToReady(beadID); err != nil {
				return nil, fmt.Errorf("expand: promote bead %s to ready: %w", beadID, err)
			}
		}
	}

	return result, nil
}

// promoteToReady switches a bead from sling:planned to sling:ready.
func promoteToReady(beadID string) error {
	if err := bead.RemoveLabel(beadID, bead.LabelPlanned); err != nil {
		return err
	}
	return bead.AddLabel(beadID, bead.LabelReady)
}

// PromoteReadyBeads checks all sling:planned beads and promotes those whose
// dependencies are all closed to sling:ready.
func PromoteReadyBeads(epicID string) error {
	planned, err := bead.List(bead.LabelPlanned)
	if err != nil {
		return fmt.Errorf("promote: list planned beads: %w", err)
	}

	for _, b := range planned {
		if b.ParentID != epicID {
			continue
		}
		allClosed, err := allDepsClosed(b.DependsOn)
		if err != nil {
			return err
		}
		if allClosed {
			if err := promoteToReady(b.ID); err != nil {
				return fmt.Errorf("promote: bead %s: %w", b.ID, err)
			}
			fmt.Printf("  Promoted bead %s to sling:ready\n", b.ID)
		}
	}
	return nil
}

// allDepsClosed returns true if every dep bead ID has status "closed".
func allDepsClosed(depIDs []string) (bool, error) {
	for _, id := range depIDs {
		closed, err := bead.IsClosed(id)
		if err != nil {
			return false, err
		}
		if !closed {
			return false, nil
		}
	}
	return true, nil
}
