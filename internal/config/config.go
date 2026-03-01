package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the merged sling configuration.
type Config struct {
	Project   ProjectConfig   `mapstructure:"project"`
	Execution ExecutionConfig `mapstructure:"execution"`
	Context   ContextConfig   `mapstructure:"context"`
	Notify    NotifyConfig    `mapstructure:"notifications"`

	// GitHub token from env or global config.
	GitHubToken string
	// Linear token from env or global config.
	LinearToken string
}

type ProjectConfig struct {
	Client      string `mapstructure:"client"`
	Remote      string `mapstructure:"remote"`
	IssueSource string `mapstructure:"issue_source"` // "github", "linear", "auto"
	LinearTeam  string `mapstructure:"linear_team"`
	GitHubRepo  string `mapstructure:"github_repo"` // auto-detected if empty
}

type ExecutionConfig struct {
	MaxAttempts    int `mapstructure:"max_attempts"`
	ReviewMaxRounds int `mapstructure:"review_max_rounds"`
}

type ContextConfig struct {
	Conventions       string `mapstructure:"conventions"`
	TechStack         string `mapstructure:"tech_stack"`
	AgentInstructions string `mapstructure:"agent_instructions"`
}

type NotifyConfig struct {
	TelegramEnabled bool   `mapstructure:"telegram_enabled"`
	TelegramChatID  string `mapstructure:"telegram_chat_id"`
}

// Load reads the global config then overlays the repo-local sling.toml.
// workdir is the repo root (usually the current directory).
func Load(workdir string) (*Config, error) {
	v := viper.New()

	// Defaults.
	v.SetDefault("execution.max_attempts", 3)
	v.SetDefault("execution.review_max_rounds", 3)
	v.SetDefault("project.issue_source", "auto")

	// Global config: ~/.config/sling/config.toml
	if home, err := os.UserHomeDir(); err == nil {
		v.SetConfigFile(filepath.Join(home, ".config", "sling", "config.toml"))
		_ = v.ReadInConfig() // ok if missing
	}

	// Repo-local overlay.
	local := viper.New()
	local.SetConfigFile(filepath.Join(workdir, "sling.toml"))
	if err := local.ReadInConfig(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading sling.toml: %w", err)
	}
	for _, k := range local.AllKeys() {
		v.Set(k, local.Get(k))
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	// Tokens from environment.
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		cfg.GitHubToken = t
	}
	if t := os.Getenv("LINEAR_API_KEY"); t != "" {
		cfg.LinearToken = t
	}

	return &cfg, nil
}
