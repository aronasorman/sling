package pipeline

import (
	"errors"
	"fmt"

	"github.com/aronasorman/sling/internal/bead"
)

// ErrCycle is returned by TopoSort when the dependency graph contains a cycle.
var ErrCycle = errors.New("dependency cycle detected")

// ExpandResult maps plan bead index → bd bead ID and records the topological layers.
type ExpandResult struct {
	EpicID       string
	BeadsByIndex map[int]string // plan index (1-based) → bead ID
	TopoLayers   [][]string     // bead IDs in topological order; TopoLayers[0] = roots
}

// package-level function variables allow tests to inject stubs without the bd binary.
var (
	expandBeadCreate = func(title, body, parentID string, labels []string) (string, error) {
		return bead.Create(title, body, parentID, labels)
	}
	expandBeadSetDependsOn = func(id string, deps []string) error {
		return bead.SetDependsOn(id, deps)
	}
	expandBeadRemoveLabel = func(id, label string) error {
		return bead.RemoveLabel(id, label)
	}
	expandBeadAddLabel = func(id, label string) error {
		return bead.AddLabel(id, label)
	}
	expandBeadList = func(label string) ([]*bead.Bead, error) {
		return bead.List(label)
	}
	expandBeadIsClosed = func(id string) (bool, error) {
		return bead.IsClosed(id)
	}
)

// TopoSort performs Kahn's topological sort on the plan-bead dependency graph.
//
// Parameters:
//
//	beads     – the full slice of PlanBeads from the Plan (order does not matter).
//	idByIndex – map from plan index (1-based int) to the bd bead ID created by Expand.
//
// Returns:
//
//	layers – bead IDs grouped by topological level.
//	         layers[0] contains IDs of beads with no dependencies (roots).
//	         layers[k] contains IDs of beads whose every dependency is in layers[0..k-1].
//	         len(layers) == 0 when beads is empty.
//	err    – wraps ErrCycle (errors.Is(err, ErrCycle) == true) if the graph has a cycle.
//	         Returns a non-nil error if idByIndex is missing an index referenced in DependsOn;
//	         in that case err does NOT wrap ErrCycle.
//
// TopoSort is a pure function: it performs no I/O and has no side effects.
func TopoSort(beads []PlanBead, idByIndex map[int]string) (layers [][]string, err error) {
	if len(beads) == 0 {
		return nil, nil
	}

	// Initialize in-degree and adjacency list for all beads.
	inDegree := make(map[int]int, len(beads))
	dependents := make(map[int][]int, len(beads)) // index → indices that depend on it

	for _, pb := range beads {
		if _, exists := inDegree[pb.Index]; !exists {
			inDegree[pb.Index] = 0
		}
	}

	// Build edges: for each bead, for each dependency it declares,
	// add a reverse edge dep→bead and increment bead's in-degree.
	for _, pb := range beads {
		for _, depIdx := range pb.DependsOn {
			if _, ok := idByIndex[depIdx]; !ok {
				return nil, fmt.Errorf("toposort: bead %d references unknown dependency index %d", pb.Index, depIdx)
			}
			inDegree[pb.Index]++
			dependents[depIdx] = append(dependents[depIdx], pb.Index)
		}
	}

	// Seed the first wave: all beads with in-degree 0.
	var currentWave []int
	for _, pb := range beads {
		if inDegree[pb.Index] == 0 {
			currentWave = append(currentWave, pb.Index)
		}
	}

	processed := 0

	for len(currentWave) > 0 {
		var layerIDs []string
		var nextWave []int

		for _, idx := range currentWave {
			id, ok := idByIndex[idx]
			if !ok {
				// Should not happen: we validated all dep indices above and
				// all beads were initialised in inDegree above.
				return nil, fmt.Errorf("toposort: index %d not found in idByIndex", idx)
			}
			layerIDs = append(layerIDs, id)
			processed++

			for _, dep := range dependents[idx] {
				inDegree[dep]--
				if inDegree[dep] == 0 {
					nextWave = append(nextWave, dep)
				}
			}
		}

		layers = append(layers, layerIDs)
		currentWave = nextWave
	}

	if processed < len(beads) {
		return nil, fmt.Errorf("toposort: %w", ErrCycle)
	}

	return layers, nil
}

// Expand creates bd child beads from the plan, wires depends_on relationships,
// runs a topological sort to detect cycles, and promotes root beads (no deps)
// to sling:ready.
//
// Returns ErrCycle (via errors.Is) if the dependency graph contains a cycle.
// In that case no sling:ready promotions are performed and no depends_on metadata
// is written (all created beads remain sling:planned; callers should close or
// delete them).
func Expand(plan *Plan, epicID string) (*ExpandResult, error) {
	result := &ExpandResult{
		EpicID:       epicID,
		BeadsByIndex: make(map[int]string),
	}

	// Pass 1: create all beads as sling:planned.
	for _, pb := range plan.Beads {
		id, err := expandBeadCreate(pb.Title, pb.Body, epicID, []string{bead.LabelPlanned})
		if err != nil {
			return nil, fmt.Errorf("expand: create bead %q: %w", pb.Title, err)
		}
		result.BeadsByIndex[pb.Index] = id
		fmt.Printf("  Created bead %d/%d: %s (id=%s)\n", pb.Index, len(plan.Beads), pb.Title, id)
	}

	// Topo sort – runs between pass 1 and pass 2 so a cycle causes an early
	// return before any SetDependsOn calls are made.
	tLayers, err := TopoSort(plan.Beads, result.BeadsByIndex)
	if err != nil {
		return nil, fmt.Errorf("expand: %w", err)
	}
	result.TopoLayers = tLayers

	// Pass 2: set depends_on relationships.
	// The inline promoteToReady call has been removed; promotion happens below.
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
			if err := expandBeadSetDependsOn(beadID, depIDs); err != nil {
				return nil, fmt.Errorf("expand: set depends_on for bead %s: %w", beadID, err)
			}
		}
	}

	// Promotion pass: promote all root beads (layer 0) to sling:ready.
	if len(result.TopoLayers) > 0 {
		for _, id := range result.TopoLayers[0] {
			if err := promoteToReady(id); err != nil {
				return nil, fmt.Errorf("expand: promote bead %s to ready: %w", id, err)
			}
		}
	}

	return result, nil
}

// promoteToReady switches a bead from sling:planned to sling:ready.
func promoteToReady(beadID string) error {
	if err := expandBeadRemoveLabel(beadID, bead.LabelPlanned); err != nil {
		return err
	}
	return expandBeadAddLabel(beadID, bead.LabelReady)
}

// PromoteReadyBeads checks all sling:planned beads and promotes those whose
// dependencies are all closed to sling:ready.
func PromoteReadyBeads(epicID string) error {
	planned, err := expandBeadList(bead.LabelPlanned)
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
		closed, err := expandBeadIsClosed(id)
		if err != nil {
			return false, err
		}
		if !closed {
			return false, nil
		}
	}
	return true, nil
}
