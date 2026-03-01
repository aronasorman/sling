package pipeline

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aronasorman/sling/internal/agent"
)

func TestSpecFile(t *testing.T) {
	f := SpecFile("bead-456")
	if f == "" {
		t.Error("SpecFile returned empty string")
	}
	// Should contain the bead ID in the filename.
	if got := filepath.Base(f); got != "sling-spec-bead-456.md" {
		t.Errorf("SpecFile base = %q; want %q", got, "sling-spec-bead-456.md")
	}
}

func TestSpecAgentSystemPrompt(t *testing.T) {
	const (
		title    = "Add Foo feature"
		body     = "Implement the Foo feature with Bar dependency."
		specFile = "/tmp/sling-spec-test.md"
	)

	prompt := agent.SpecAgentSystemPrompt(title, body, specFile, nil)

	// Must identify the SpecAgent role.
	if !strings.Contains(prompt, "SpecAgent") {
		t.Error("prompt should mention SpecAgent")
	}

	// Must include the bead title.
	if !strings.Contains(prompt, title) {
		t.Errorf("prompt should include bead title %q", title)
	}

	// Must include the bead body.
	if !strings.Contains(prompt, body) {
		t.Errorf("prompt should include bead body %q", body)
	}

	// Must reference the output spec file path.
	if !strings.Contains(prompt, specFile) {
		t.Errorf("prompt should include spec file path %q", specFile)
	}

	// Must include the required spec sections.
	for _, section := range []string{
		"Implementation approach",
		"Interface / API contracts",
		"Test plan",
		"Acceptance criteria",
	} {
		if !strings.Contains(prompt, section) {
			t.Errorf("prompt should include section %q", section)
		}
	}

	// Must instruct the agent not to print to stdout.
	if !strings.Contains(prompt, "Do NOT print") {
		t.Error("prompt should instruct agent not to print the spec to stdout")
	}
}

func TestSpecAgentSystemPromptWithContextFiles(t *testing.T) {
	ctx := map[string]string{
		"conventions": "Use camelCase for all identifiers.",
		"tech-stack":  "Go 1.24, Cobra CLI.",
	}

	prompt := agent.SpecAgentSystemPrompt("Some bead", "Some body", "/tmp/spec.md", ctx)

	for name, content := range ctx {
		if !strings.Contains(prompt, name) {
			t.Errorf("prompt should include context file name %q", name)
		}
		if !strings.Contains(prompt, content) {
			t.Errorf("prompt should include context file content %q", content)
		}
	}
}
