package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPlan(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "plan.json")
		content := `{
  "epic_title": "Test Epic",
  "beads": [
    {"index": 1, "title": "Bead 1", "body": "Do thing 1", "depends_on": []},
    {"index": 2, "title": "Bead 2", "body": "Do thing 2", "depends_on": [1]}
  ]
}`
		if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		plan, err := ReadPlan(f)
		if err != nil {
			t.Fatalf("ReadPlan: %v", err)
		}
		if plan.EpicTitle != "Test Epic" {
			t.Errorf("EpicTitle = %q; want %q", plan.EpicTitle, "Test Epic")
		}
		if len(plan.Beads) != 2 {
			t.Fatalf("len(Beads) = %d; want 2", len(plan.Beads))
		}
		if plan.Beads[1].DependsOn[0] != 1 {
			t.Errorf("Beads[1].DependsOn[0] = %d; want 1", plan.Beads[1].DependsOn[0])
		}
	})

	t.Run("JSON wrapped in markdown fences", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "plan.json")
		content := "```json\n{\"epic_title\": \"Fenced\", \"beads\": [{\"index\": 1, \"title\": \"B\", \"body\": \"\", \"depends_on\": []}]}\n```"
		if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		plan, err := ReadPlan(f)
		if err != nil {
			t.Fatalf("ReadPlan with fences: %v", err)
		}
		if plan.EpicTitle != "Fenced" {
			t.Errorf("EpicTitle = %q; want %q", plan.EpicTitle, "Fenced")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := ReadPlan("/nonexistent/plan.json")
		if err == nil {
			t.Error("expected error for missing file")
		}
	})
}

func TestPlanFile(t *testing.T) {
	f := PlanFile("epic-123")
	if f == "" {
		t.Error("PlanFile returned empty string")
	}
	// Should contain the epic ID.
	if got := filepath.Base(f); got != "sling-plan-epic-123.json" {
		t.Errorf("PlanFile base = %q; want %q", got, "sling-plan-epic-123.json")
	}
}
