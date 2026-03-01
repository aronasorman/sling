package pipeline

import (
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/aronasorman/sling/internal/bead"
)

// ── TopoSort unit tests (TC-1 through TC-10) ─────────────────────────────────

// TC-1: Empty plan
func TestTopoSort_Empty(t *testing.T) {
	layers, err := TopoSort([]PlanBead{}, map[int]string{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(layers) != 0 {
		t.Fatalf("expected empty layers, got %v", layers)
	}
}

// TC-2: Single bead, no deps
func TestTopoSort_SingleNoDeps(t *testing.T) {
	beads := []PlanBead{{Index: 1, DependsOn: []int{}}}
	idByIndex := map[int]string{1: "a"}

	layers, err := TopoSort(beads, idByIndex)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("expected 1 layer, got %d: %v", len(layers), layers)
	}
	if len(layers[0]) != 1 || layers[0][0] != "a" {
		t.Errorf("expected layers[0]=[\"a\"], got %v", layers[0])
	}
}

// TC-3: Linear chain A→B→C
func TestTopoSort_LinearChain(t *testing.T) {
	beads := []PlanBead{
		{Index: 1, DependsOn: []int{}},
		{Index: 2, DependsOn: []int{1}},
		{Index: 3, DependsOn: []int{2}},
	}
	idByIndex := map[int]string{1: "a", 2: "b", 3: "c"}

	layers, err := TopoSort(beads, idByIndex)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(layers) != 3 {
		t.Fatalf("expected 3 layers, got %d: %v", len(layers), layers)
	}
	if len(layers[0]) != 1 || layers[0][0] != "a" {
		t.Errorf("expected layers[0]=[\"a\"], got %v", layers[0])
	}
	if len(layers[1]) != 1 || layers[1][0] != "b" {
		t.Errorf("expected layers[1]=[\"b\"], got %v", layers[1])
	}
	if len(layers[2]) != 1 || layers[2][0] != "c" {
		t.Errorf("expected layers[2]=[\"c\"], got %v", layers[2])
	}
}

// TC-4: Diamond A→B, A→C, B→D, C→D
func TestTopoSort_Diamond(t *testing.T) {
	beads := []PlanBead{
		{Index: 1, DependsOn: []int{}},
		{Index: 2, DependsOn: []int{1}},
		{Index: 3, DependsOn: []int{1}},
		{Index: 4, DependsOn: []int{2, 3}},
	}
	idByIndex := map[int]string{1: "a", 2: "b", 3: "c", 4: "d"}

	layers, err := TopoSort(beads, idByIndex)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(layers) != 3 {
		t.Fatalf("expected 3 layers, got %d: %v", len(layers), layers)
	}
	if len(layers[0]) != 1 || layers[0][0] != "a" {
		t.Errorf("expected layers[0]=[\"a\"], got %v", layers[0])
	}

	// layers[1] must contain "b" and "c" (order unspecified)
	l1 := sorted(layers[1])
	if len(l1) != 2 || l1[0] != "b" || l1[1] != "c" {
		t.Errorf("expected layers[1] to contain \"b\" and \"c\", got %v", layers[1])
	}

	if len(layers[2]) != 1 || layers[2][0] != "d" {
		t.Errorf("expected layers[2]=[\"d\"], got %v", layers[2])
	}
}

// TC-5: Two independent roots
func TestTopoSort_TwoRoots(t *testing.T) {
	beads := []PlanBead{
		{Index: 1, DependsOn: []int{}},
		{Index: 2, DependsOn: []int{}},
		{Index: 3, DependsOn: []int{1, 2}},
	}
	idByIndex := map[int]string{1: "a", 2: "b", 3: "c"}

	layers, err := TopoSort(beads, idByIndex)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %d: %v", len(layers), layers)
	}

	l0 := sorted(layers[0])
	if len(l0) != 2 || l0[0] != "a" || l0[1] != "b" {
		t.Errorf("expected layers[0] to contain \"a\" and \"b\", got %v", layers[0])
	}
	if len(layers[1]) != 1 || layers[1][0] != "c" {
		t.Errorf("expected layers[1]=[\"c\"], got %v", layers[1])
	}
}

// TC-6: Simple cycle A→B→A
func TestTopoSort_SimpleCycle(t *testing.T) {
	beads := []PlanBead{
		{Index: 1, DependsOn: []int{2}},
		{Index: 2, DependsOn: []int{1}},
	}
	idByIndex := map[int]string{1: "a", 2: "b"}

	layers, err := TopoSort(beads, idByIndex)
	if layers != nil {
		t.Errorf("expected nil layers, got %v", layers)
	}
	if !errors.Is(err, ErrCycle) {
		t.Errorf("expected ErrCycle, got %v", err)
	}
}

// TC-7: Self-loop
func TestTopoSort_SelfLoop(t *testing.T) {
	beads := []PlanBead{
		{Index: 1, DependsOn: []int{1}},
	}
	idByIndex := map[int]string{1: "a"}

	layers, err := TopoSort(beads, idByIndex)
	if layers != nil {
		t.Errorf("expected nil layers, got %v", layers)
	}
	if !errors.Is(err, ErrCycle) {
		t.Errorf("expected ErrCycle, got %v", err)
	}
}

// TC-8: Three-node cycle in larger graph (node A is a root, but B/C/D form a cycle)
func TestTopoSort_CycleInLargerGraph(t *testing.T) {
	beads := []PlanBead{
		{Index: 1, DependsOn: []int{}},   // A – root, fine
		{Index: 2, DependsOn: []int{3}},  // B → C
		{Index: 3, DependsOn: []int{4}},  // C → D
		{Index: 4, DependsOn: []int{2}},  // D → B (closes cycle)
	}
	idByIndex := map[int]string{1: "a", 2: "b", 3: "c", 4: "d"}

	layers, err := TopoSort(beads, idByIndex)
	if layers != nil {
		t.Errorf("expected nil layers, got %v", layers)
	}
	if !errors.Is(err, ErrCycle) {
		t.Errorf("expected ErrCycle, got %v", err)
	}
}

// TC-9: Unknown dependency index
func TestTopoSort_UnknownDependencyIndex(t *testing.T) {
	beads := []PlanBead{
		{Index: 1, DependsOn: []int{99}},
	}
	idByIndex := map[int]string{1: "a"} // index 99 absent

	layers, err := TopoSort(beads, idByIndex)
	if layers != nil {
		t.Errorf("expected nil layers, got %v", layers)
	}
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if errors.Is(err, ErrCycle) {
		t.Errorf("expected error NOT to be ErrCycle, but errors.Is(err, ErrCycle)==true")
	}
}

// TC-10: All beads independent (no edges)
func TestTopoSort_AllIndependent(t *testing.T) {
	beads := []PlanBead{
		{Index: 1, DependsOn: []int{}},
		{Index: 2, DependsOn: []int{}},
		{Index: 3, DependsOn: []int{}},
	}
	idByIndex := map[int]string{1: "a", 2: "b", 3: "c"}

	layers, err := TopoSort(beads, idByIndex)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("expected 1 layer, got %d: %v", len(layers), layers)
	}
	l0 := sorted(layers[0])
	if len(l0) != 3 || l0[0] != "a" || l0[1] != "b" || l0[2] != "c" {
		t.Errorf("expected layers[0] to contain \"a\", \"b\", \"c\", got %v", layers[0])
	}
}

// ── Expand integration / behaviour tests (TC-11 through TC-14) ───────────────
//
// These tests inject stubs via the package-level function variables so they do
// not require the bd binary on PATH.

// fakeBeadEnv sets up stub implementations of the bead operations used by
// Expand/PromoteReadyBeads and restores the originals when the test finishes.
type fakeBeadEnv struct {
	created      []string   // IDs returned from Create, in order
	promoted     []string   // IDs passed to promoteToReady (via AddLabel sling:ready)
	removedLabel []string   // IDs passed to RemoveLabel
	depsSet      map[string][]string // beadID → dep IDs passed to SetDependsOn
	nextID       int
}

func newFakeBeadEnv(t *testing.T) *fakeBeadEnv {
	t.Helper()
	f := &fakeBeadEnv{
		depsSet: make(map[string][]string),
		nextID:  1,
	}

	orig := struct {
		create      func(string, string, string, []string) (string, error)
		setDeps     func(string, []string) error
		removeLabel func(string, string) error
		addLabel    func(string, string) error
		list        func(string) ([]*bead.Bead, error)
		isClosed    func(string) (bool, error)
	}{
		create:      expandBeadCreate,
		setDeps:     expandBeadSetDependsOn,
		removeLabel: expandBeadRemoveLabel,
		addLabel:    expandBeadAddLabel,
		list:        expandBeadList,
		isClosed:    expandBeadIsClosed,
	}

	expandBeadCreate = func(title, body, parentID string, labels []string) (string, error) {
		id := fmt.Sprintf("bead-%d", f.nextID)
		f.nextID++
		f.created = append(f.created, id)
		return id, nil
	}
	expandBeadSetDependsOn = func(id string, deps []string) error {
		f.depsSet[id] = append(f.depsSet[id], deps...)
		return nil
	}
	expandBeadRemoveLabel = func(id, label string) error {
		f.removedLabel = append(f.removedLabel, id)
		return nil
	}
	expandBeadAddLabel = func(id, label string) error {
		if label == bead.LabelReady {
			f.promoted = append(f.promoted, id)
		}
		return nil
	}

	t.Cleanup(func() {
		expandBeadCreate = orig.create
		expandBeadSetDependsOn = orig.setDeps
		expandBeadRemoveLabel = orig.removeLabel
		expandBeadAddLabel = orig.addLabel
		expandBeadList = orig.list
		expandBeadIsClosed = orig.isClosed
	})

	return f
}

// TC-11: Expand with no-dep beads calls promoteToReady for all roots
func TestExpand_NoDepsPromotesRoots(t *testing.T) {
	f := newFakeBeadEnv(t)

	plan := &Plan{
		EpicTitle: "test",
		Beads: []PlanBead{
			{Index: 1, Title: "A", DependsOn: []int{}},
			{Index: 2, Title: "B", DependsOn: []int{}},
			{Index: 3, Title: "C", DependsOn: []int{1, 2}},
		},
	}

	result, err := Expand(plan, "epic-1")
	if err != nil {
		t.Fatalf("Expand returned unexpected error: %v", err)
	}

	// All 3 beads created
	if len(f.created) != 3 {
		t.Errorf("expected 3 beads created, got %d: %v", len(f.created), f.created)
	}

	id1 := result.BeadsByIndex[1]
	id2 := result.BeadsByIndex[2]
	id3 := result.BeadsByIndex[3]

	// SetDependsOn called once for bead 3 with IDs of beads 1 and 2
	if len(f.depsSet) != 1 {
		t.Errorf("expected SetDependsOn called once, got %d calls: %v", len(f.depsSet), f.depsSet)
	}
	deps3 := sorted(f.depsSet[id3])
	want3 := sorted([]string{id1, id2})
	if fmt.Sprint(deps3) != fmt.Sprint(want3) {
		t.Errorf("SetDependsOn for bead3: got %v, want %v", deps3, want3)
	}

	// promoteToReady called for beads 1 and 2 only
	promoted := sorted(f.promoted)
	wantPromoted := sorted([]string{id1, id2})
	if fmt.Sprint(promoted) != fmt.Sprint(wantPromoted) {
		t.Errorf("promoted: got %v, want %v", promoted, wantPromoted)
	}

	// TopoLayers[0] contains ids 1 and 2 (any order)
	if len(result.TopoLayers) < 2 {
		t.Fatalf("expected at least 2 topo layers, got %d", len(result.TopoLayers))
	}
	layer0 := sorted(result.TopoLayers[0])
	wantLayer0 := sorted([]string{id1, id2})
	if fmt.Sprint(layer0) != fmt.Sprint(wantLayer0) {
		t.Errorf("TopoLayers[0]: got %v, want %v", layer0, wantLayer0)
	}

	// TopoLayers[1] contains id3
	if len(result.TopoLayers[1]) != 1 || result.TopoLayers[1][0] != id3 {
		t.Errorf("TopoLayers[1]: got %v, want [%s]", result.TopoLayers[1], id3)
	}
}

// TC-12: Expand with cycle returns ErrCycle and does NOT call SetDependsOn or promoteToReady
func TestExpand_CycleReturnsErrCycle(t *testing.T) {
	f := newFakeBeadEnv(t)

	plan := &Plan{
		EpicTitle: "cycle test",
		Beads: []PlanBead{
			{Index: 1, Title: "A", DependsOn: []int{2}},
			{Index: 2, Title: "B", DependsOn: []int{1}},
		},
	}

	_, err := Expand(plan, "epic-2")
	if !errors.Is(err, ErrCycle) {
		t.Errorf("expected ErrCycle, got %v", err)
	}

	// Both beads were created (first pass ran)
	if len(f.created) != 2 {
		t.Errorf("expected 2 beads created in first pass, got %d", len(f.created))
	}

	// SetDependsOn must NOT have been called (topo sort failed before pass 2)
	if len(f.depsSet) != 0 {
		t.Errorf("expected SetDependsOn not called, but got %v", f.depsSet)
	}

	// promoteToReady must NOT have been called
	if len(f.promoted) != 0 {
		t.Errorf("expected no promotions, but got %v", f.promoted)
	}
}

// TC-13: Expand linear chain – only first bead is promoted
func TestExpand_LinearChainOnlyFirstPromoted(t *testing.T) {
	f := newFakeBeadEnv(t)

	plan := &Plan{
		EpicTitle: "linear",
		Beads: []PlanBead{
			{Index: 1, Title: "A", DependsOn: []int{}},
			{Index: 2, Title: "B", DependsOn: []int{1}},
			{Index: 3, Title: "C", DependsOn: []int{2}},
		},
	}

	result, err := Expand(plan, "epic-3")
	if err != nil {
		t.Fatalf("Expand returned unexpected error: %v", err)
	}

	id1 := result.BeadsByIndex[1]

	// Only bead 1 promoted
	if len(f.promoted) != 1 || f.promoted[0] != id1 {
		t.Errorf("expected only bead1 (%s) promoted, got %v", id1, f.promoted)
	}

	// TopoLayers == [[id1],[id2],[id3]]
	id2 := result.BeadsByIndex[2]
	id3 := result.BeadsByIndex[3]
	if len(result.TopoLayers) != 3 {
		t.Fatalf("expected 3 topo layers, got %d: %v", len(result.TopoLayers), result.TopoLayers)
	}
	if len(result.TopoLayers[0]) != 1 || result.TopoLayers[0][0] != id1 {
		t.Errorf("TopoLayers[0]: got %v, want [%s]", result.TopoLayers[0], id1)
	}
	if len(result.TopoLayers[1]) != 1 || result.TopoLayers[1][0] != id2 {
		t.Errorf("TopoLayers[1]: got %v, want [%s]", result.TopoLayers[1], id2)
	}
	if len(result.TopoLayers[2]) != 1 || result.TopoLayers[2][0] != id3 {
		t.Errorf("TopoLayers[2]: got %v, want [%s]", result.TopoLayers[2], id3)
	}
}

// TC-14: ExpandResult.TopoLayers populated on success
func TestExpand_TopoLayersPopulated(t *testing.T) {
	newFakeBeadEnv(t)

	plan := &Plan{
		EpicTitle: "any acyclic plan",
		Beads: []PlanBead{
			{Index: 1, Title: "A", DependsOn: []int{}},
			{Index: 2, Title: "B", DependsOn: []int{1}},
		},
	}

	result, err := Expand(plan, "epic-4")
	if err != nil {
		t.Fatalf("Expand returned unexpected error: %v", err)
	}

	if len(result.TopoLayers) == 0 {
		t.Fatal("expected TopoLayers to be non-empty")
	}

	// Every bead ID appears exactly once across all layers.
	seen := make(map[string]int)
	for _, layer := range result.TopoLayers {
		for _, id := range layer {
			seen[id]++
		}
	}
	for id, count := range seen {
		if count != 1 {
			t.Errorf("bead ID %s appears %d times in TopoLayers (want 1)", id, count)
		}
	}
	if len(seen) != len(plan.Beads) {
		t.Errorf("expected %d unique bead IDs in TopoLayers, got %d", len(plan.Beads), len(seen))
	}
}

// ── Regression: PromoteReadyBeads unchanged (TC-15) ──────────────────────────

// TC-15: PromoteReadyBeads still promotes a bead when all its deps are closed
func TestPromoteReadyBeads_PromotesBadWhenDepsClosed(t *testing.T) {
	const epicID = "epic-99"

	// Stub list: return one sling:planned bead that belongs to our epic,
	// with one dependency.
	planBead := &bead.Bead{
		ID:        "bead-planned",
		ParentID:  epicID,
		Labels:    []string{bead.LabelPlanned},
		DependsOn: []string{"bead-dep"},
	}

	origList := expandBeadList
	origIsClosed := expandBeadIsClosed
	origRemove := expandBeadRemoveLabel
	origAdd := expandBeadAddLabel
	t.Cleanup(func() {
		expandBeadList = origList
		expandBeadIsClosed = origIsClosed
		expandBeadRemoveLabel = origRemove
		expandBeadAddLabel = origAdd
	})

	expandBeadList = func(label string) ([]*bead.Bead, error) {
		if label == bead.LabelPlanned {
			return []*bead.Bead{planBead}, nil
		}
		return nil, nil
	}
	expandBeadIsClosed = func(id string) (bool, error) {
		if id == "bead-dep" {
			return true, nil // dep is closed
		}
		return false, nil
	}

	var promoted []string
	expandBeadRemoveLabel = func(id, label string) error {
		return nil
	}
	expandBeadAddLabel = func(id, label string) error {
		if label == bead.LabelReady {
			promoted = append(promoted, id)
		}
		return nil
	}

	if err := PromoteReadyBeads(epicID); err != nil {
		t.Fatalf("PromoteReadyBeads returned error: %v", err)
	}

	if len(promoted) != 1 || promoted[0] != "bead-planned" {
		t.Errorf("expected \"bead-planned\" promoted, got %v", promoted)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// sorted returns a sorted copy of ss.
func sorted(ss []string) []string {
	cp := make([]string, len(ss))
	copy(cp, ss)
	sort.Strings(cp)
	return cp
}
