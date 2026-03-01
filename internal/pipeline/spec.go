package pipeline

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aronasorman/sling/internal/agent"
	"github.com/aronasorman/sling/internal/bead"
)

// SpecFile returns the path where the spec file is written.
// The SpecAgent writes Markdown to this file.
func SpecFile(beadID string) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("sling-spec-%s.md", beadID))
}

// RunSpecAgent invokes the SpecAgent (Sonnet) to write a detailed technical
// specification for the given bead. The spec is written to
// /tmp/sling-spec-<beadID>.md by the agent. After the agent completes, the
// file is read and its contents returned.
//
// repoRoot is passed as the agent's working directory so it can explore the
// codebase and understand existing patterns before writing the spec.
func RunSpecAgent(beadID, repoRoot string, contextFiles map[string]string) (string, error) {
	b, err := bead.Show(beadID)
	if err != nil {
		return "", fmt.Errorf("spec: fetch bead %s: %w", beadID, err)
	}

	specFile := SpecFile(beadID)

	// Remove any stale spec file.
	_ = os.Remove(specFile)

	systemPrompt := agent.SpecAgentSystemPrompt(b.Title, b.Body, specFile, contextFiles)
	userPrompt := fmt.Sprintf(
		"Write a technical spec for bead: %s\n\nDescription:\n%s\n\nWrite the spec to %s now.",
		b.Title, b.Body, specFile,
	)

	fmt.Printf("Running SpecAgent (Sonnet) for bead %s...\n", beadID)
	// REVIEW: MaxTurns is hard-coded to 20 with no way to configure it. For complex
	// beads on large codebases the spec agent may need more turns to fully explore
	// the codebase before writing. Consider exposing this via a SpecOptions struct
	// or reusing the MaxAttempts/MaxTurns field from ExecuteOptions.
	//
	// REVIEW: WorkDir is repoRoot, but the agent.Run framework always passes
	// --dangerously-skip-permissions. The spec system prompt says "Explore the
	// codebase … Write the file and then stop" but never says "do NOT modify
	// source files". A confused agent could mutate the main repo rather than the
	// isolated worktree. Add an explicit rule: "Do NOT modify any source file in
	// the repository."
	if err := agent.Run(agent.RunOptions{
		WorkDir:      repoRoot,
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Model:        agent.ModelSonnet,
		MaxTurns:     20,
	}); err != nil {
		return "", fmt.Errorf("spec: spec agent: %w", err)
	}

	data, err := os.ReadFile(specFile)
	if err != nil {
		return "", fmt.Errorf("spec: read spec file %s: %w", specFile, err)
	}
	return string(data), nil
}
