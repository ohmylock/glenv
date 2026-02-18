package main

import (
	"testing"

	"github.com/ohmylock/glenv/pkg/config"
)

func cfg(envs map[string]config.EnvironmentConfig) *config.Config {
	return &config.Config{Environments: envs}
}

func TestResolveEnvFile(t *testing.T) {
	tests := []struct {
		name        string
		flagFile    string
		environment string
		cfg         *config.Config
		want        string
	}{
		{
			name:        "explicit flag wins over config",
			flagFile:    "custom.env",
			environment: "staging",
			cfg:         cfg(map[string]config.EnvironmentConfig{"staging": {File: "staging.env"}}),
			want:        "custom.env",
		},
		{
			name:        "config env file used when flag empty",
			flagFile:    "",
			environment: "staging",
			cfg:         cfg(map[string]config.EnvironmentConfig{"staging": {File: "staging.env"}}),
			want:        "staging.env",
		},
		{
			name:        "fallback to .env when environment is *",
			flagFile:    "",
			environment: "*",
			cfg:         cfg(map[string]config.EnvironmentConfig{"staging": {File: "staging.env"}}),
			want:        ".env",
		},
		{
			name:        "fallback to .env when no matching env in config",
			flagFile:    "",
			environment: "production",
			cfg:         cfg(map[string]config.EnvironmentConfig{"staging": {File: "staging.env"}}),
			want:        ".env",
		},
		{
			name:        "fallback to .env when matching env has empty File",
			flagFile:    "",
			environment: "staging",
			cfg:         cfg(map[string]config.EnvironmentConfig{"staging": {File: ""}}),
			want:        ".env",
		},
		{
			name:        "fallback to .env when Environments is nil",
			flagFile:    "",
			environment: "staging",
			cfg:         cfg(nil),
			want:        ".env",
		},
		{
			name:        "explicit flag wins even when environment is *",
			flagFile:    "override.env",
			environment: "*",
			cfg:         cfg(nil),
			want:        "override.env",
		},
		{
			name:        "wildcard scope ignores cfg.Environments[*] entry",
			flagFile:    "",
			environment: "*",
			cfg:         cfg(map[string]config.EnvironmentConfig{"*": {File: "global.env"}}),
			want:        ".env",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveEnvFile(tt.flagFile, tt.environment, tt.cfg)
			if got != tt.want {
				t.Errorf("resolveEnvFile(%q, %q, cfg) = %q, want %q", tt.flagFile, tt.environment, got, tt.want)
			}
		})
	}
}

func TestResolveWorkers(t *testing.T) {
	tests := []struct {
		name    string
		global  *GlobalOptions
		cfg     *config.Config
		want    int
	}{
		{
			name:   "CLI flag takes priority",
			global: &GlobalOptions{Workers: 3},
			cfg:    &config.Config{RateLimit: config.RateLimitConfig{MaxConcurrent: 10}},
			want:   3,
		},
		{
			name:   "config MaxConcurrent used when CLI flag is 0",
			global: &GlobalOptions{Workers: 0},
			cfg:    &config.Config{RateLimit: config.RateLimitConfig{MaxConcurrent: 7}},
			want:   7,
		},
		{
			name:   "default 5 when both are 0",
			global: &GlobalOptions{Workers: 0},
			cfg:    &config.Config{RateLimit: config.RateLimitConfig{MaxConcurrent: 0}},
			want:   5,
		},
		{
			name:   "CLI flag 1 is explicit, not treated as zero",
			global: &GlobalOptions{Workers: 1},
			cfg:    &config.Config{RateLimit: config.RateLimitConfig{MaxConcurrent: 10}},
			want:   1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveWorkers(tt.global, tt.cfg)
			if got != tt.want {
				t.Errorf("resolveWorkers() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMaskIfNeeded(t *testing.T) {
	tests := []struct {
		name           string
		value          string
		classification string
		want           string
	}{
		{
			name:           "masks when classification contains masked",
			value:          "secret123",
			classification: "masked",
			want:           "***",
		},
		{
			name:           "masks when classification is masked+protected",
			value:          "secret123",
			classification: "masked,protected",
			want:           "***",
		},
		{
			name:           "does not mask when classification is protected only",
			value:          "value",
			classification: "protected",
			want:           "value",
		},
		{
			name:           "does not mask when classification is empty",
			value:          "value",
			classification: "",
			want:           "value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskIfNeeded(tt.value, tt.classification)
			if got != tt.want {
				t.Errorf("maskIfNeeded(%q, %q) = %q, want %q", tt.value, tt.classification, got, tt.want)
			}
		})
	}
}

func TestBuildTags(t *testing.T) {
	tests := []struct {
		name           string
		classification string
		want           string
	}{
		{
			name:           "empty when no classification",
			classification: "",
			want:           "",
		},
		{
			name:           "masked tag",
			classification: "masked",
			want:           " [masked]",
		},
		{
			name:           "protected tag",
			classification: "protected",
			want:           " [protected]",
		},
		{
			name:           "both tags",
			classification: "masked,protected",
			want:           " [masked] [protected]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTags(tt.classification)
			if got != tt.want {
				t.Errorf("buildTags(%q) = %q, want %q", tt.classification, got, tt.want)
			}
		})
	}
}
