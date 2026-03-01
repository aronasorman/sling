package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aronasorman/sling/internal/agent"
	"github.com/aronasorman/sling/internal/issue"
)

// PlanBead is a single planned bead from the Planner's output.
type PlanBead struct {
	Index     int    `json:"index"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	DependsOn []int  `json:"depends_on"` // indices (1-based)
}

// Plan is the Planner's full output.
type Plan struct {
	EpicTitle string     `json:"epic_title"`
	Beads     []PlanBead `json:"beads"`
}

// PlanFile returns the path where the plan file is written.
// Per issue #4: write to /tmp/sling-plan-<epicID>.json
func PlanFile(epicID string) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("sling-plan-%s.json", epicID))
}

// RunPlanner invokes the Planner (Opus) agent to decompose the epic.
// The plan is written to /tmp/sling-plan-<epicID>.json by the agent.
// After the agent completes, the file is read and returned.
func RunPlanner(epicID string, iss *issue.Issue, contextFiles map[string]string) (*Plan, error) {
	planFile := PlanFile(epicID)

	// Remove any stale plan file.
	_ = os.Remove(planFile)

	systemPrompt := agent.PlannerSystemPrompt(iss.Title, iss.Body, planFile, contextFiles)
	userPrompt := fmt.Sprintf(
		"Decompose this epic into atomic beads.\n\nTitle: %s\n\nDescription:\n%s\n\nWrite the plan to %s now.",
		iss.Title, iss.Body, planFile,
	)

	fmt.Printf("Running Planner (Opus) for epic %s...\n", epicID)
	if err := agent.Run(agent.RunOptions{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Model:        agent.ModelOpus,
		MaxTurns:     20,
	}); err != nil {
		return nil, fmt.Errorf("plan: planner agent: %w", err)
	}

	return ReadPlan(planFile)
}

// ReadPlan reads and parses a plan file.
func ReadPlan(planFile string) (*Plan, error) {
	data, err := os.ReadFile(planFile)
	if err != nil {
		return nil, fmt.Errorf("plan: read plan file %s: %w", planFile, err)
	}

	// Strip any markdown code fences in case the model wrapped JSON.
	raw := strings.TrimSpace(string(data))
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		// Drop first and last lines (``` ... ```)
		if len(lines) > 2 {
			raw = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	var plan Plan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return nil, fmt.Errorf("plan: decode plan JSON: %w (content: %s)", err, raw[:min(200, len(raw))])
	}
	return &plan, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
