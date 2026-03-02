package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aronasorman/sling/internal/config"
	"github.com/aronasorman/sling/internal/issue"
	"github.com/aronasorman/sling/internal/pipeline"
)

var startRig string

var startCmd = &cobra.Command{
	Use:   "start <issue-ref-or-title>",
	Short: "Fetch an issue (or expand a title), create an epic bead, plan and expand it into child beads",
	Long: `Start creates an epic bead and decomposes it into child beads via the Planner.

Normal mode: <issue-ref> is a GitHub issue number, Linear ID, or similar.
  sling start 42
  sling start LIN-123

Gastown mode: when run inside a gastown workspace (or with --rig), the argument
is treated as a plain title. No external issue tracker is required.
  sling start "add user authentication"
  sling start --rig sling "add user authentication"

In gastown mode, beads are created in the specified rig.`,
	Args: cobra.ExactArgs(1),
	RunE: runStart,
}

func init() {
	startCmd.Flags().StringVar(&startRig, "rig", "", "Gastown rig name to create beads in (auto-detected if inside a gastown workspace)")
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

	// Determine rig: explicit --rig flag overrides auto-detection.
	rig := startRig
	if rig == "" {
		rig = detectGastownRig(cwd)
	}

	var src issue.Source
	if rig != "" {
		// Gastown mode: treat the argument as a plain title, no tracker needed.
		fmt.Printf("Gastown mode: rig=%s, title=%q\n", rig, ref)
		src = issue.NewDescriptionSource(ref, "")
	} else {
		// Normal mode: resolve issue from the configured tracker.
		githubRepo := cfg.Project.GitHubRepo
		if githubRepo == "" {
			detected, err := detectGitHubRepo(cwd)
			if err != nil {
				fmt.Printf("Warning: %v\n", err)
			} else {
				githubRepo = detected
			}
		}

		// Fail fast: if the issue source is GitHub we must have a repo slug.
		if cfg.Project.IssueSource == "github" && githubRepo == "" {
			return fmt.Errorf("start: github_repo is required when issue_source=github; set it in sling.toml or ensure the git remote is a GitHub URL")
		}

		src, err = issue.DetectSource(cfg.Project.IssueSource, ref, cfg.GitHubToken, cfg.LinearToken, githubRepo)
		if err != nil {
			return err
		}
	}

	ctx := context.Background()

	// Phase 1: Intake.
	result, err := pipeline.Intake(ctx, ref, src, pipeline.IntakeOpts{Rig: rig})
	if err != nil {
		return err
	}

	// Phase 2: Planning.
	contextFiles := loadContextFiles(cfg, cwd)
	plan, err := pipeline.RunPlanner(result.EpicID, cwd, result.Issue, contextFiles)
	if err != nil {
		return fmt.Errorf("planning failed: %w", err)
	}
	fmt.Printf("Plan: %d beads\n", len(plan.Beads))

	// Phase 3: Expansion.
	expandResult, err := pipeline.Expand(plan, result.EpicID, pipeline.ExpandOpts{Rig: rig})
	if err != nil {
		return fmt.Errorf("expansion failed: %w", err)
	}
	_ = expandResult

	fmt.Printf("\nDone. Epic bead: %s\nRun `sling next` to start executing.\n", result.EpicID)
	return nil
}

// detectGastownRig walks up from cwd looking for a gastown rig config.json.
// Returns the rig name if found, empty string if not in a gastown workspace.
func detectGastownRig(cwd string) string {
	dir := cwd
	for {
		configPath := filepath.Join(dir, "config.json")
		if data, err := os.ReadFile(configPath); err == nil {
			var rigConfig struct {
				Type string `json:"type"`
				Name string `json:"name"`
			}
			if json.Unmarshal(data, &rigConfig) == nil && rigConfig.Type == "rig" && rigConfig.Name != "" {
				return rigConfig.Name
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// detectGitHubRepo tries to infer "owner/repo" from the git remote URL.
// Returns an error if the remote URL cannot be fetched or is not a GitHub URL.
func detectGitHubRepo(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", fmt.Errorf("detectGitHubRepo: could not get git remote URL: %w", err)
	}
	rawURL := strings.TrimSpace(string(out))
	repo := parseGitHubRepoFromURL(rawURL)
	if repo == "" {
		return "", fmt.Errorf("detectGitHubRepo: remote %q is not a GitHub URL; set github_repo in sling.toml", rawURL)
	}
	return repo, nil
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
		// Support absolute paths (e.g. /tmp/foo.md) and repo-relative paths.
		if !filepath.IsAbs(path) {
			path = filepath.Join(repoRoot, path)
		}
		data, err := os.ReadFile(path)
		if err == nil {
			files[name] = string(data)
		}
	}
	read("conventions", cfg.Context.Conventions)
	read("tech_stack", cfg.Context.TechStack)
	read("agent_instructions", cfg.Context.AgentInstructions)

	// Issue #5: load address-review skill if available.
	skillPath := filepath.Join(os.Getenv("HOME"), ".claude", "skills", "address-review", "SKILL.md")
	if data, err := os.ReadFile(skillPath); err == nil {
		files["address-review-skill"] = string(data)
	}

	return files
}
