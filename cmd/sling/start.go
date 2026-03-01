package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aronasorman/sling/internal/config"
	"github.com/aronasorman/sling/internal/issue"
	"github.com/aronasorman/sling/internal/pipeline"
)

var startCmd = &cobra.Command{
	Use:   "start <issue-ref>",
	Short: "Fetch an issue, create an epic bead, plan and expand it into child beads",
	Args:  cobra.ExactArgs(1),
	RunE:  runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	ref := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	// Resolve GitHub repo from config or git remote.
	githubRepo := cfg.Project.GitHubRepo
	if githubRepo == "" {
		githubRepo = detectGitHubRepo(cwd)
	}

	src, err := issue.DetectSource(cfg.Project.IssueSource, ref, cfg.GitHubToken, cfg.LinearToken, githubRepo)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Phase 1: Intake.
	result, err := pipeline.Intake(ctx, ref, src)
	if err != nil {
		return err
	}

	// Phase 2: Planning.
	contextFiles := loadContextFiles(cfg, cwd)
	plan, err := pipeline.RunPlanner(result.EpicID, result.Issue, contextFiles)
	if err != nil {
		return fmt.Errorf("planning failed: %w", err)
	}
	fmt.Printf("Plan: %d beads\n", len(plan.Beads))

	// Phase 3: Expansion.
	expandResult, err := pipeline.Expand(plan, result.EpicID)
	if err != nil {
		return fmt.Errorf("expansion failed: %w", err)
	}
	_ = expandResult

	fmt.Printf("\nDone. Epic bead: %s\nRun `sling next` to start executing.\n", result.EpicID)
	return nil
}

// detectGitHubRepo tries to infer "owner/repo" from the git remote URL.
func detectGitHubRepo(dir string) string {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return parseGitHubRepoFromURL(strings.TrimSpace(string(out)))
}

// parseGitHubRepoFromURL extracts "owner/repo" from a GitHub remote URL.
// Handles both HTTPS (https://github.com/owner/repo.git) and SSH (git@github.com:owner/repo.git).
func parseGitHubRepoFromURL(url string) string {
	url = strings.TrimSuffix(url, ".git")

	// SSH: git@github.com:owner/repo
	if strings.HasPrefix(url, "git@github.com:") {
		return strings.TrimPrefix(url, "git@github.com:")
	}

	// HTTPS: https://github.com/owner/repo
	if idx := strings.Index(url, "github.com/"); idx != -1 {
		return url[idx+len("github.com/"):]
	}

	return ""
}

// loadContextFiles reads the configured context files into a map.
func loadContextFiles(cfg *config.Config, repoRoot string) map[string]string {
	files := make(map[string]string)
	read := func(name, path string) {
		if path == "" {
			return
		}
		data, err := os.ReadFile(repoRoot + "/" + path)
		if err == nil {
			files[name] = string(data)
		}
	}
	read("conventions", cfg.Context.Conventions)
	read("tech_stack", cfg.Context.TechStack)
	read("agent_instructions", cfg.Context.AgentInstructions)

	// Issue #5: load address-review skill if available.
	skillPath := os.Getenv("HOME") + "/.claude/skills/address-review/SKILL.md"
	if data, err := os.ReadFile(skillPath); err == nil {
		files["address-review-skill"] = string(data)
	}

	return files
}
