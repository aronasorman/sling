package agent

import (
	"strings"
	"testing"
)

// TC-A1: EpicExecutorSystemPrompt includes all sub-bead titles and no signal-done instruction.
func TestEpicExecutorSystemPrompt_IncludesSubBeadTitles(t *testing.T) {
	subBeads := []EpicSubBeadSpec{
		{ID: "s1", Title: "Add login", Body: "implement login endpoint", Spec: ""},
		{ID: "s2", Title: "Add logout", Body: "implement logout endpoint", Spec: "spec content"},
	}

	result := EpicExecutorSystemPrompt("e1", "Add auth", "auth epic body", subBeads, nil)

	// Must include sub-bead titles.
	if !strings.Contains(result, "Add login") {
		t.Error("expected result to contain 'Add login'")
	}
	if !strings.Contains(result, "Add logout") {
		t.Error("expected result to contain 'Add logout'")
	}

	// Must include spec content when provided.
	if !strings.Contains(result, "spec content") {
		t.Error("expected result to contain 'spec content' from s2.Spec")
	}

	// Must NOT instruct the agent to call sling signal-done.
	if strings.Contains(result, "sling signal-done") {
		t.Error("expected result NOT to contain 'sling signal-done'")
	}

	// Must include the epic ID.
	if !strings.Contains(result, "e1") {
		t.Error("expected result to contain epic ID 'e1'")
	}
}

// TC-A2: EpicExecutorSystemPrompt includes context files.
func TestEpicExecutorSystemPrompt_IncludesContextFiles(t *testing.T) {
	subBeads := []EpicSubBeadSpec{
		{ID: "s1", Title: "A bead", Body: "some body", Spec: ""},
	}
	contextFiles := map[string]string{
		"agent_instructions": "follow these rules",
	}

	result := EpicExecutorSystemPrompt("e1", "Epic", "epic body", subBeads, contextFiles)

	if !strings.Contains(result, "follow these rules") {
		t.Error("expected result to contain context file content 'follow these rules'")
	}
}

// EpicExecutorSystemPrompt omits Technical spec section when Spec is empty.
func TestEpicExecutorSystemPrompt_NoSpecWhenEmpty(t *testing.T) {
	subBeads := []EpicSubBeadSpec{
		{ID: "s1", Title: "A bead", Body: "body", Spec: ""},
	}

	result := EpicExecutorSystemPrompt("e1", "Epic", "body", subBeads, nil)

	if strings.Contains(result, "Technical spec") {
		t.Error("expected result NOT to contain 'Technical spec' when Spec is empty")
	}
}
