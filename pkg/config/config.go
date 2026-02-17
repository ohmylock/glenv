package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// GitLabConfig holds GitLab connection settings.
type GitLabConfig struct {
	URL       string `yaml:"url"`
	Token     string `yaml:"token"`
	ProjectID string `yaml:"project_id"`
}

// RateLimitConfig holds rate limiting and retry settings.
type RateLimitConfig struct {
	RequestsPerSecond   float64       `yaml:"requests_per_second"`
	MaxConcurrent       int           `yaml:"max_concurrent"`
	RetryMax            int           `yaml:"retry_max"`
	RetryInitialBackoff time.Duration `yaml:"retry_initial_backoff"`
}

// EnvironmentConfig defines a named deployment environment.
type EnvironmentConfig struct {
	File      string `yaml:"file"`
	Protected bool   `yaml:"protected"`
}

// ClassifyConfig holds user-supplied classification rule overrides.
type ClassifyConfig struct {
	MaskedPatterns []string `yaml:"masked_patterns"`
	MaskedExclude  []string `yaml:"masked_exclude"`
	FilePatterns   []string `yaml:"file_patterns"`
	FileExclude    []string `yaml:"file_exclude"`
}

// Config is the root configuration structure.
type Config struct {
	GitLab       GitLabConfig                 `yaml:"gitlab"`
	RateLimit    RateLimitConfig              `yaml:"rate_limit"`
	Environments map[string]EnvironmentConfig `yaml:"environments"`
	Classify     ClassifyConfig               `yaml:"classify"`
}

// defaults returns a Config populated with built-in default values.
func defaults() Config {
	return Config{
		GitLab: GitLabConfig{
			URL: "https://gitlab.com",
		},
		RateLimit: RateLimitConfig{
			RequestsPerSecond:   10,
			MaxConcurrent:       5,
			RetryMax:            3,
			RetryInitialBackoff: time.Second,
		},
	}
}

// applyEnvVars overlays GITLAB_* environment variables onto cfg.
// Only non-empty env vars overwrite existing values.
func applyEnvVars(cfg *Config) {
	if v := os.Getenv("GITLAB_TOKEN"); v != "" {
		cfg.GitLab.Token = v
	}
	if v := os.Getenv("GITLAB_PROJECT_ID"); v != "" {
		cfg.GitLab.ProjectID = v
	}
	if v := os.Getenv("GITLAB_URL"); v != "" {
		cfg.GitLab.URL = v
	}
}

// expandEnvVars runs os.ExpandEnv on all string fields in cfg.
func expandEnvVars(cfg *Config) {
	cfg.GitLab.URL = os.ExpandEnv(cfg.GitLab.URL)
	cfg.GitLab.Token = os.ExpandEnv(cfg.GitLab.Token)
	cfg.GitLab.ProjectID = os.ExpandEnv(cfg.GitLab.ProjectID)
}

// resolveConfigPath determines the config file path to use.
//
// Priority:
//  1. override (explicit --config path) — must exist or error
//  2. searchDir/.glenv.yml
//  3. ~/.glenv.yml
//
// searchDir defaults to the current working directory if empty.
// Returns "" (no error) if no config file is found via automatic search.
func resolveConfigPath(override, searchDir string) (string, error) {
	if override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("config: explicit path not found: %w", err)
		}
		return override, nil
	}

	if searchDir == "" {
		var err error
		searchDir, err = os.Getwd()
		if err != nil {
			searchDir = "."
		}
	}

	local := filepath.Join(searchDir, ".glenv.yml")
	if _, err := os.Stat(local); err == nil {
		return local, nil
	}

	home, err := os.UserHomeDir()
	if err == nil {
		homeConfig := filepath.Join(home, ".glenv.yml")
		if _, err := os.Stat(homeConfig); err == nil {
			return homeConfig, nil
		}
	}

	return "", nil
}

// Load builds a Config using the priority chain:
// defaults → env vars → YAML file → env var expansion.
//
// configPath is the explicit config file path (e.g. from --config flag).
// If empty, the automatic search chain is used.
func Load(configPath string) (*Config, error) {
	// Start with defaults.
	cfg := defaults()

	// Overlay env vars.
	applyEnvVars(&cfg)

	// Resolve config file.
	resolved, err := resolveConfigPath(configPath, "")
	if err != nil {
		return nil, err
	}

	// Parse and overlay YAML file (YAML wins over env vars).
	if resolved != "" {
		data, err := os.ReadFile(resolved)
		if err != nil {
			return nil, fmt.Errorf("config: read %q: %w", resolved, err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("config: parse %q: %w", resolved, err)
		}
	}

	// Expand ${VAR} references in string fields.
	expandEnvVars(&cfg)

	return &cfg, nil
}

// Validate checks that required fields are set.
func (c *Config) Validate() error {
	if c.GitLab.Token == "" {
		return errors.New("config: gitlab.token is required (set GITLAB_TOKEN or token in config file)")
	}
	if c.GitLab.ProjectID == "" {
		return errors.New("config: gitlab.project_id is required (set GITLAB_PROJECT_ID or project_id in config file)")
	}
	return nil
}
