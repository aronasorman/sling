// Package agent wraps the Claude Code CLI for different agent roles.
// Each role gets its own system prompt and is run as a separate session.
package agent

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Role identifies an agent's purpose.
type Role string

const (
	RolePlanner   Role = "planner"
	RoleExecutor  Role = "executor"
	RoleReviewer  Role = "reviewer"
	RoleAddresser Role = "addresser"
)

// RunOptions configures a Claude Code agent run.
type RunOptions struct {
	// WorkDir is the working directory for the agent.
	WorkDir string
	// SystemPrompt is the system prompt for this agent.
	SystemPrompt string
	// UserPrompt is the initial user message.
	UserPrompt string
	// MaxTurns limits the number of agentic turns (0 = unlimited).
	MaxTurns int
	// Model to use (e.g. "claude-opus-4-6", "claude-sonnet-4-6").
	Model string
	// Env is a map of additional environment variables to set for the agent.
	Env map[string]string
}

// Run executes the claude CLI with the given options.
// It streams output to stdout/stderr and returns an error if the agent fails.
func Run(opts RunOptions) error {
	args := []string{
		"--print",                         // non-interactive
		"--output-format", "text",
		"--dangerously-skip-permissions",  // sling always runs in a trusted repo context
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", opts.MaxTurns))
	}
	if opts.SystemPrompt != "" {
		args = append(args, "--system-prompt", opts.SystemPrompt)
	}
	// The user prompt is passed as the final positional argument.
	args = append(args, opts.UserPrompt)

	cmd := exec.Command("claude", args...)
	cmd.Dir = opts.WorkDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	for k, v := range opts.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	if err := cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("claude agent (%s): exit %d", opts.WorkDir, e.ExitCode())
		}
		return fmt.Errorf("claude agent: %w", err)
	}
	return nil
}

// Model constants matching ARCHITECTURE.md.
const (
	ModelOpus   = "claude-opus-4-6"
	ModelSonnet = "claude-sonnet-4-6"
)

// PlannerSystemPrompt returns the system prompt for the Planner agent.
// epicTitle and epicBody describe the epic. planFile is the output path.
func PlannerSystemPrompt(epicTitle, epicBody, planFile string, contextFiles map[string]string) string {
	var sb strings.Builder
	sb.WriteString("You are Sling's Planner agent. Your job is to decompose an epic into atomic, independently-implementable beads.\n\n")
	sb.WriteString("## Output format\n\n")
	sb.WriteString(fmt.Sprintf("Write your plan as JSON to the file `%s`. Do NOT print JSON to stdout.\n\n", planFile))
	sb.WriteString("The JSON must match this schema:\n")
	sb.WriteString(`{
  "epic_title": "string",
  "beads": [
    {
      "index": 1,
      "title": "string",
      "body": "string (markdown, describe what to implement and how to test)",
      "depends_on": [2, 3]  // indices of beads this one depends on (empty array if none)
    }
  ]
}
`)
	sb.WriteString("\n## Rules\n\n")
	sb.WriteString("- Each bead must be a small, independently testable unit of work.\n")
	sb.WriteString("- depends_on must form a DAG (no cycles). Index from 1.\n")
	sb.WriteString("- Maximum 10 beads per epic. Prefer fewer, larger beads over many tiny ones.\n")
	sb.WriteString("- Write the file and then stop.\n\n")

	if len(contextFiles) > 0 {
		sb.WriteString("## Project context\n\n")
		for name, content := range contextFiles {
			sb.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", name, content))
		}
	}

	return sb.String()
}

// ExecutorSystemPrompt returns the system prompt for the Executor agent.
// beadTitle and beadBody describe the task.
func ExecutorSystemPrompt(beadTitle, beadBody, beadID string, contextFiles map[string]string) string {
	var sb strings.Builder
	sb.WriteString("You are Sling's Executor agent. Your job is to implement the bead described below.\n\n")
	sb.WriteString(fmt.Sprintf("## Bead: %s\n\n%s\n\n", beadTitle, beadBody))
	sb.WriteString("## Rules\n\n")
	sb.WriteString("- Write code, tests, and any necessary documentation.\n")
	sb.WriteString("- Run the tests and ensure they pass before finishing.\n")
	sb.WriteString("- Commit your changes with jj (do NOT push).\n\n")

	if len(contextFiles) > 0 {
		sb.WriteString("## Project context\n\n")
		for name, content := range contextFiles {
			sb.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", name, content))
		}
	}

	sb.WriteString("## Done condition\n\n")
	sb.WriteString("When all work is committed and tests pass, you are done. The orchestrator handles all signaling automatically.\n")
	sb.WriteString(fmt.Sprintf("The bead ID for this task is: %s\n", beadID))

	return sb.String()
}

// ReviewerSystemPrompt returns the system prompt for the Reviewer agent.
// It reviews the diff and adds REVIEW: markers.
func ReviewerSystemPrompt(beadTitle string, contextFiles map[string]string) string {
	var sb strings.Builder
	sb.WriteString("You are Sling's Reviewer agent. Your job is to adversarially review the implementation of the bead below.\n\n")
	sb.WriteString(fmt.Sprintf("## Bead: %s\n\n", beadTitle))
	sb.WriteString("## Review process\n\n")
	sb.WriteString("1. Run `jj diff` to see all changes.\n")
	sb.WriteString("2. Examine the code thoroughly. Look for bugs, missing tests, security issues, style violations.\n")
	sb.WriteString("3. For each issue, add a REVIEW: marker comment directly in the relevant file.\n")
	sb.WriteString("   Use the appropriate comment syntax for the language:\n")
	sb.WriteString("   - Go/JS/TS/Rust/C: `// REVIEW: <description>`\n")
	sb.WriteString("   - Python/Shell: `# REVIEW: <description>`\n")
	sb.WriteString("   - SQL: `-- REVIEW: <description>`\n")
	sb.WriteString("   - HTML: `<!-- REVIEW: <description> -->`\n")
	sb.WriteString("4. Create a commit with `jj commit -m 'review: <brief summary>'`.\n")
	sb.WriteString("5. If the implementation is clean and correct, write nothing and exit.\n\n")
	sb.WriteString("## Rules\n\n")
	sb.WriteString("- Be thorough and adversarial. Your job is to find problems.\n")
	sb.WriteString("- Only add REVIEW: markers for real issues. Don't nit-pick style that doesn't matter.\n\n")

	if len(contextFiles) > 0 {
		sb.WriteString("## Project context\n\n")
		for name, content := range contextFiles {
			sb.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", name, content))
		}
	}

	return sb.String()
}

// SpecAgentSystemPrompt returns the system prompt for the SpecAgent.
// beadTitle and beadBody describe the bead. specFile is the output path.
func SpecAgentSystemPrompt(beadTitle, beadBody, specFile string, contextFiles map[string]string) string {
	var sb strings.Builder
	sb.WriteString("You are Sling's SpecAgent. Your job is to write a detailed technical specification for the bead described below.\n\n")
	sb.WriteString(fmt.Sprintf("## Bead: %s\n\n%s\n\n", beadTitle, beadBody))
	sb.WriteString("## Output\n\n")
	sb.WriteString(fmt.Sprintf("Write your spec as Markdown to the file `%s`. Do NOT print the spec to stdout.\n\n", specFile))
	sb.WriteString("The spec must include:\n\n")
	sb.WriteString("1. **Implementation approach** — high-level design and key decisions.\n")
	sb.WriteString("2. **Interface / API contracts** — function signatures, types, return values, errors.\n")
	sb.WriteString("3. **Test plan** — specific test cases with inputs and expected outputs.\n")
	sb.WriteString("4. **Acceptance criteria** — checklist of conditions that must be true when the bead is done.\n\n")
	sb.WriteString("## Rules\n\n")
	sb.WriteString("- Be concrete and specific. The Executor will implement exactly what you specify.\n")
	sb.WriteString("- Include enough detail that an Executor agent can implement without ambiguity.\n")
	sb.WriteString("- Explore the codebase to understand existing patterns before writing the spec.\n")
	sb.WriteString("- Do NOT modify any source file in the repository. Read-only exploration only.\n")
	sb.WriteString("- Write the file and then stop.\n\n")

	if len(contextFiles) > 0 {
		sb.WriteString("## Project context\n\n")
		for name, content := range contextFiles {
			sb.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", name, content))
		}
	}

	return sb.String()
}

// EpicSubBeadSpec holds data for one sub-bead in the epic executor prompt.
type EpicSubBeadSpec struct {
	ID    string // bead ID
	Title string
	Body  string
	Spec  string // spec agent output, or "" if not available
}

// EpicExecutorSystemPrompt builds the system prompt for the epic Executor agent.
// subBeads must be in topological order (dependencies first).
// epicID is included as an informational reference only; the orchestrator
// handles all label transitions — the agent does NOT call sling signal-done.
func EpicExecutorSystemPrompt(
	epicID, epicTitle, epicBody string,
	subBeads []EpicSubBeadSpec,
	contextFiles map[string]string,
) string {
	var sb strings.Builder
	sb.WriteString("You are Sling's Executor agent. Your job is to implement all beads in the epic described below.\n\n")
	sb.WriteString(fmt.Sprintf("## Epic: %s (ID: %s)\n\n%s\n\n", epicTitle, epicID, epicBody))
	sb.WriteString("## Beads to implement (implement in the order listed; dependencies are pre-resolved)\n\n")

	for i, sp := range subBeads {
		sb.WriteString(fmt.Sprintf("### Bead %d: %s (ID: %s)\n\n%s\n\n", i+1, sp.Title, sp.ID, sp.Body))
		if sp.Spec != "" {
			sb.WriteString("#### Technical spec\n\n")
			sb.WriteString(sp.Spec)
			sb.WriteString("\n\n")
		}
	}

	sb.WriteString("## Rules\n\n")
	sb.WriteString("- Implement each bead in the exact order listed above.\n")
	sb.WriteString("- After implementing each bead, commit with jj:\n")
	sb.WriteString("    jj commit -m \"feat: <bead title>\"\n")
	sb.WriteString("- Run `go test ./...` (or the project's test command) after each bead.\n")
	sb.WriteString("- All beads share this single worktree and branch; do NOT create separate branches.\n")
	sb.WriteString("- The orchestrator handles all signaling automatically when you are done.\n\n")

	if len(contextFiles) > 0 {
		sb.WriteString("## Project context\n\n")
		for name, content := range contextFiles {
			sb.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", name, content))
		}
	}

	sb.WriteString("## Done condition\n\n")
	sb.WriteString("When all beads are implemented, all tests pass, and all changes are committed, you are done.\n")
	sb.WriteString(fmt.Sprintf("Epic ID: %s\n", epicID))

	return sb.String()
}

// EpicReviewerSystemPrompt returns the system prompt for holistic epic review.
// The Reviewer is given the full epic context (title, body, sub-bead specs) and
// is instructed to look for cross-bead integration issues in addition to
// per-file defects.
//
// Parameters:
//
//	epicTitle    – displayed as the review target
//	epicBody     – the original epic description / acceptance criteria
//	subBeads     – ordered list of sub-beads with their specs (may be empty)
//	contextFiles – additional project context (same map used by other prompts)
func EpicReviewerSystemPrompt(
	epicTitle, epicBody string,
	subBeads []EpicSubBeadSpec,
	contextFiles map[string]string,
) string {
	var sb strings.Builder
	sb.WriteString("You are Sling's Reviewer agent. Your job is to holistically review the\n")
	sb.WriteString("implementation of the epic described below.\n\n")
	sb.WriteString(fmt.Sprintf("## Epic: %s\n\n%s\n\n", epicTitle, epicBody))

	if len(subBeads) > 0 {
		sb.WriteString("## Sub-beads implemented (for context)\n\n")
		for i, sp := range subBeads {
			sb.WriteString(fmt.Sprintf("### Sub-bead %d: %s (ID: %s)\n\n%s\n\n", i+1, sp.Title, sp.ID, sp.Body))
			if sp.Spec != "" {
				sb.WriteString("#### Technical spec\n\n")
				sb.WriteString(sp.Spec)
				sb.WriteString("\n\n")
			}
		}
	}

	sb.WriteString("## Review process\n\n")
	sb.WriteString("1. Run `jj diff` to see all changes across the entire epic.\n")
	sb.WriteString("2. Examine each file thoroughly. Look for:\n")
	sb.WriteString("   - Bugs, missing tests, security issues, style violations (per-file).\n")
	sb.WriteString("   - Integration issues: do the sub-beads fit together correctly?\n")
	sb.WriteString("   - API consistency: naming, return types, error patterns uniform?\n")
	sb.WriteString("   - Missing cross-bead integration tests.\n")
	sb.WriteString("   - Architectural drift: does any sub-bead violate the overall design?\n")
	sb.WriteString("3. For each issue, add a REVIEW: marker comment directly in the relevant file.\n")
	sb.WriteString("   Use the appropriate comment syntax for the language:\n")
	sb.WriteString("   - Go/JS/TS/Rust/C: `// REVIEW: <description>`\n")
	sb.WriteString("   - Python/Shell:    `# REVIEW: <description>`\n")
	sb.WriteString("   - SQL:             `-- REVIEW: <description>`\n")
	sb.WriteString("   - HTML:            `<!-- REVIEW: <description> -->`\n")
	sb.WriteString("4. Create a commit with `jj commit -m 'review: <brief summary>'`.\n")
	sb.WriteString("5. If the implementation is clean and correct, write nothing and exit.\n\n")
	sb.WriteString("## Rules\n\n")
	sb.WriteString("- Be thorough and adversarial. Your job is to find problems.\n")
	sb.WriteString("- Consider the sub-beads as a whole — not just individually.\n")
	sb.WriteString("- Only add REVIEW: markers for real issues. Don't nit-pick style that doesn't matter.\n\n")

	if len(contextFiles) > 0 {
		sb.WriteString("## Project context\n\n")
		for name, content := range contextFiles {
			sb.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", name, content))
		}
	}

	return sb.String()
}

// AddresserSystemPrompt returns the system prompt for the Addresser agent.
// It resolves all REVIEW: markers in the worktree.
func AddresserSystemPrompt(beadTitle string, contextFiles map[string]string) string {
	var sb strings.Builder
	sb.WriteString("You are Sling's Addresser agent. Your job is to resolve all REVIEW: markers left by the Reviewer.\n\n")
	sb.WriteString(fmt.Sprintf("## Bead: %s\n\n", beadTitle))
	sb.WriteString("## Process\n\n")

	// Issue #5: reference the address-review skill.
	if skillContent, ok := contextFiles["address-review-skill"]; ok {
		sb.WriteString("Use the address-review skill:\n\n")
		sb.WriteString(skillContent)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("1. Find all REVIEW: markers: `grep -rn 'REVIEW:' .`\n")
		sb.WriteString("2. For each marker:\n")
		sb.WriteString("   a. Understand the concern.\n")
		sb.WriteString("   b. Fix the code.\n")
		sb.WriteString("   c. Remove the REVIEW: comment.\n")
		sb.WriteString("3. Run tests and ensure they pass.\n")
		sb.WriteString("4. Commit the fixes with `jj commit -m 'address review'`.\n")
		sb.WriteString("5. Verify no REVIEW: markers remain: `grep -rn 'REVIEW:' . | grep -v '.git'`\n\n")
	}

	sb.WriteString("## Done condition\n\n")
	sb.WriteString("You are done when:\n")
	sb.WriteString("- No REVIEW: markers remain in any file (verified by grep)\n")
	sb.WriteString("- All tests pass\n")

	return sb.String()
}
