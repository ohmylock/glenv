# Configuration Reference

## Config File Locations

glenv searches for configuration in this order:

1. Path specified via `--config` flag
2. `.glenv.yml` in the current directory
3. `~/.glenv.yml` in the home directory

The first file found is used. Config file is optional — glenv works with just environment variables.

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `GITLAB_TOKEN` | Personal Access Token with `api` scope | Yes (if not in config) |
| `GITLAB_PROJECT_ID` | Project ID or URL-encoded path (e.g., `group%2Fproject`) | Yes (if not in config) |
| `GITLAB_URL` | GitLab instance URL | No (default: `https://gitlab.com`) |
| `NO_COLOR` | Disable colored output (any value) | No |

## Full Config Reference

```yaml
# =============================================================================
# glenv configuration
# =============================================================================

# GitLab connection settings
gitlab:
  # GitLab instance URL
  # For gitlab.com: https://gitlab.com (default)
  # For self-hosted: https://gitlab.company.com
  url: https://gitlab.com

  # Personal Access Token with 'api' scope
  # Supports environment variable expansion: ${GITLAB_TOKEN}
  # Create at: https://gitlab.com/-/user_settings/personal_access_tokens
  token: ${GITLAB_TOKEN}

  # Project ID (number) or URL-encoded path
  # Find at: Project → Settings → General → Project ID
  # Example path: "my-group%2Fmy-project"
  project_id: "12345678"

# Rate limiting configuration
# Defaults are conservative and safe for gitlab.com SaaS
rate_limit:
  # Maximum API requests per second
  # gitlab.com allows ~33 req/sec (2000/min), default 10 is safe
  # Self-hosted: adjust based on your instance limits
  requests_per_second: 10

  # Number of concurrent worker goroutines
  # Workers share the rate limiter, so this doesn't increase API rate
  # More workers = better throughput when rate allows
  max_concurrent: 5

  # Maximum retry attempts per failed operation
  retry_max: 3

  # Initial backoff duration before first retry
  # Subsequent retries use exponential backoff: duration * 2^attempt
  retry_initial_backoff: 1s

# Environment definitions for --all sync
environments:
  # Each key is the GitLab environment_scope name
  production:
    # Path to .env file (relative to config file or absolute)
    file: deploy/gitlab-envs/.env.production
    # Mark secret variables as protected in this environment
    # Protected variables are only available in protected branches/environments
    protected: true

  staging:
    file: deploy/gitlab-envs/.env.staging
    protected: false

  # You can define any number of environments
  # development:
  #   file: deploy/gitlab-envs/.env.development
  #   protected: false

# Variable classification rules
# These extend the built-in defaults
classify:
  # Key substrings that indicate a variable should be masked
  # Masked variables have their values hidden in GitLab UI and logs
  # Note: GitLab requires masked values to be >= 8 chars and single-line
  masked_patterns:
    - "_TOKEN"
    - "TOKEN_"
    - "SECRET"
    - "PASSWORD"
    - "API_KEY"
    - "PRIVATE_KEY"
    - "CREDENTIAL"
    - "DSN"
    - "ENCRYPTION"

  # Key substrings that should NOT be masked (exceptions)
  masked_exclude:
    - "MAX_TOKENS"
    - "TIMEOUT"
    - "PORT"
    - "HOST"
    - "URL"
    - "ENABLED"
    - "DEBUG"
    - "LEVEL"
    - "COUNT"
    - "SIZE"
    - "LIMIT"

  # Key substrings for file-type variables
  # File variables: GitLab writes content to a temp file, passes the file path
  file_patterns:
    - "PRIVATE_KEY"
    - "_CERT"
    - "_PEM"

  # Key substrings excluded from file type detection
  file_exclude:
    - "_PATH"
    - "_DIR"
    - "_URL"
```

## Priority

Settings are resolved in this order (highest priority first):

1. **CLI flags** (`--token`, `--workers`, etc.)
2. **Environment variables** (`GITLAB_TOKEN`, etc.)
3. **Config file** (`.glenv.yml`)
4. **Built-in defaults**

## Environment Variable Expansion

String values in the config file support `${VAR}` syntax for environment variable expansion:

```yaml
gitlab:
  token: ${GITLAB_TOKEN}           # expanded from environment
  project_id: ${MY_PROJECT_ID}     # expanded from environment
```

If the referenced environment variable is not set, the value remains as the literal string `${VAR}`.

## Creating a GitLab Token

1. Go to GitLab → **Settings** → **Access Tokens**
   - gitlab.com: https://gitlab.com/-/user_settings/personal_access_tokens
   - Self-hosted: `https://your-gitlab.com/-/user_settings/personal_access_tokens`
2. Create a token with the **`api`** scope
3. Copy the token (starts with `glpat-`)
4. Store it securely (password manager, environment variable)

## Finding Project ID

- Go to your project page in GitLab
- **Settings** → **General** → look for **Project ID** (a number)
- Or use the URL-encoded path: `my-group%2Fmy-project`
