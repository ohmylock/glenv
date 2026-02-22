package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	// No config file, no env vars → pure defaults
	clearGitLabEnv(t)

	cfg, err := Load("")
	require.NoError(t, err)

	assert.Equal(t, "https://gitlab.com", cfg.GitLab.URL)
	assert.Equal(t, "", cfg.GitLab.Token)
	assert.Equal(t, "", cfg.GitLab.ProjectID)
	assert.Equal(t, float64(10), cfg.RateLimit.RequestsPerSecond)
	assert.Equal(t, 5, cfg.RateLimit.MaxConcurrent)
	assert.Equal(t, 3, cfg.RateLimit.RetryMax)
	assert.Equal(t, time.Second, cfg.RateLimit.RetryInitialBackoff)
}

func TestLoad_EnvVars(t *testing.T) {
	clearGitLabEnv(t)
	t.Setenv("GITLAB_TOKEN", "mytoken")
	t.Setenv("GITLAB_PROJECT_ID", "99999")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")

	cfg, err := Load("")
	require.NoError(t, err)

	assert.Equal(t, "mytoken", cfg.GitLab.Token)
	assert.Equal(t, "99999", cfg.GitLab.ProjectID)
	assert.Equal(t, "https://gitlab.example.com", cfg.GitLab.URL)
}

func TestLoad_ConfigFile(t *testing.T) {
	clearGitLabEnv(t)

	yaml := `
gitlab:
  url: https://gitlab.mycompany.com
  token: file-token-value
  project_id: "12345"
rate_limit:
  requests_per_second: 20
  max_concurrent: 10
  retry_max: 5
  retry_initial_backoff: 2s
`
	path := writeTempConfig(t, yaml)

	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "https://gitlab.mycompany.com", cfg.GitLab.URL)
	assert.Equal(t, "file-token-value", cfg.GitLab.Token)
	assert.Equal(t, "12345", cfg.GitLab.ProjectID)
	assert.Equal(t, float64(20), cfg.RateLimit.RequestsPerSecond)
	assert.Equal(t, 10, cfg.RateLimit.MaxConcurrent)
	assert.Equal(t, 5, cfg.RateLimit.RetryMax)
	assert.Equal(t, 2*time.Second, cfg.RateLimit.RetryInitialBackoff)
}

func TestLoad_EnvExpansion(t *testing.T) {
	clearGitLabEnv(t)
	t.Setenv("GITLAB_TOKEN", "expanded-token-value")

	yaml := `
gitlab:
  token: ${GITLAB_TOKEN}
  project_id: "55555"
`
	path := writeTempConfig(t, yaml)

	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "expanded-token-value", cfg.GitLab.Token)
}

func TestLoad_EnvExpansion_EnvironmentFile(t *testing.T) {
	clearGitLabEnv(t)
	t.Setenv("STAGING_ENV_FILE", ".env.staging")

	yaml := `
gitlab:
  token: tok
  project_id: "1"
environments:
  staging:
    file: ${STAGING_ENV_FILE}
`
	path := writeTempConfig(t, yaml)

	cfg, err := Load(path)
	require.NoError(t, err)

	require.Contains(t, cfg.Environments, "staging")
	assert.Equal(t, ".env.staging", cfg.Environments["staging"].File)
}

func TestLoad_EnvExpansion_ProjectID(t *testing.T) {
	clearGitLabEnv(t)
	t.Setenv("MY_PROJECT", "77777")

	yaml := `
gitlab:
  token: sometoken
  project_id: ${MY_PROJECT}
`
	path := writeTempConfig(t, yaml)

	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "77777", cfg.GitLab.ProjectID)
}

func TestLoad_MissingConfigPath(t *testing.T) {
	_, err := Load("/nonexistent/path/.glenv.yml")
	assert.Error(t, err)
}

func TestLoad_ConfigFile_Environments(t *testing.T) {
	clearGitLabEnv(t)

	yaml := `
gitlab:
  token: tok
  project_id: "1"
environments:
  production:
    file: .env.production
  staging:
    file: .env.staging
`
	path := writeTempConfig(t, yaml)

	cfg, err := Load(path)
	require.NoError(t, err)

	require.Contains(t, cfg.Environments, "production")
	assert.Equal(t, ".env.production", cfg.Environments["production"].File)

	require.Contains(t, cfg.Environments, "staging")
	assert.Equal(t, ".env.staging", cfg.Environments["staging"].File)
}

func TestLoad_ConfigFile_ClassifyRules(t *testing.T) {
	clearGitLabEnv(t)

	yaml := `
gitlab:
  token: tok
  project_id: "1"
classify:
  masked_patterns:
    - CUSTOM_SECRET
  masked_exclude:
    - CUSTOM_IGNORE
  file_patterns:
    - CUSTOM_FILE
  file_exclude:
    - CUSTOM_PATH
`
	path := writeTempConfig(t, yaml)

	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Contains(t, cfg.Classify.MaskedPatterns, "CUSTOM_SECRET")
	assert.Contains(t, cfg.Classify.MaskedExclude, "CUSTOM_IGNORE")
	assert.Contains(t, cfg.Classify.FilePatterns, "CUSTOM_FILE")
	assert.Contains(t, cfg.Classify.FileExclude, "CUSTOM_PATH")
}

func TestLoad_EnvVars_OverrideConfigFile(t *testing.T) {
	// Env vars have higher priority than YAML config (Load order: defaults → YAML → env vars).
	clearGitLabEnv(t)
	t.Setenv("GITLAB_TOKEN", "env-token")
	t.Setenv("GITLAB_URL", "https://env.gitlab.com")

	yaml := `
gitlab:
  url: https://file.gitlab.com
  token: file-token
  project_id: "1"
`
	path := writeTempConfig(t, yaml)

	cfg, err := Load(path)
	require.NoError(t, err)

	// Env vars win over YAML file.
	assert.Equal(t, "env-token", cfg.GitLab.Token)
	assert.Equal(t, "https://env.gitlab.com", cfg.GitLab.URL)
	// Values not set via env var still come from YAML.
	assert.Equal(t, "1", cfg.GitLab.ProjectID)
}

// TestValidate_MissingToken checks that Validate returns error if token is empty.
func TestValidate_MissingToken(t *testing.T) {
	cfg := &Config{}
	cfg.GitLab.ProjectID = "12345"
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token")
}

// TestValidate_MissingProject checks that Validate returns error if project_id is empty.
func TestValidate_MissingProject(t *testing.T) {
	cfg := &Config{}
	cfg.GitLab.Token = "sometoken"
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project")
}

func TestValidate_Valid(t *testing.T) {
	cfg := &Config{}
	cfg.GitLab.Token = "tok"
	cfg.GitLab.ProjectID = "123"
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestResolveConfigPath_LocalFile(t *testing.T) {
	// Create a temp dir with a .glenv.yml and change to it
	dir := t.TempDir()
	localPath := filepath.Join(dir, ".glenv.yml")
	require.NoError(t, os.WriteFile(localPath, []byte("gitlab:\n  token: t\n"), 0600))

	// Override working directory lookup by using explicit path
	// resolveConfigPath("") should find local .glenv.yml
	// We test via Load with empty path in temp dir context by writing the file
	// and passing its parent dir as the search root
	path, err := resolveConfigPath("", dir)
	require.NoError(t, err)
	assert.Equal(t, localPath, path)
}

func TestResolveConfigPath_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	explicitPath := filepath.Join(dir, "custom.yml")
	require.NoError(t, os.WriteFile(explicitPath, []byte("gitlab:\n  token: t\n"), 0600))

	path, err := resolveConfigPath(explicitPath, dir)
	require.NoError(t, err)
	assert.Equal(t, explicitPath, path)
}

func TestResolveConfigPath_ExplicitNotFound(t *testing.T) {
	_, err := resolveConfigPath("/nonexistent/custom.yml", "")
	assert.Error(t, err)
}

func TestResolveConfigPath_NoFileFound(t *testing.T) {
	// Empty dir with no .glenv.yml → no file found, return empty path (not an error)
	dir := t.TempDir()
	path, err := resolveConfigPath("", dir)
	require.NoError(t, err)
	assert.Equal(t, "", path)
}

// --- helpers ---

func clearGitLabEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"GITLAB_TOKEN", "GITLAB_PROJECT_ID", "GITLAB_URL"} {
		t.Setenv(key, "") // set to empty so applyEnvVars skips it; t.Setenv restores original on cleanup
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTempConfig(t, "invalid: yaml: {bad")

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "glenv-*.yml")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(f.Name()) })
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}
