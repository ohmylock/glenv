<p align="center">
  <img src="https://img.shields.io/badge/go-1.25+-00ADD8?style=flat&logo=go" alt="Go version 1.25 or higher">
  <a href="https://github.com/ohmylock/glenv/actions/workflows/ci.yml"><img src="https://github.com/ohmylock/glenv/actions/workflows/ci.yml/badge.svg" alt="CI Build Status"></a>
  <a href="https://github.com/ohmylock/glenv/releases"><img src="https://img.shields.io/github/v/release/ohmylock/glenv?include_prereleases" alt="Latest Release Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="MIT License"></a>
  <a href="https://goreportcard.com/report/github.com/ohmylock/glenv"><img src="https://goreportcard.com/badge/github.com/ohmylock/glenv" alt="Go Report Card Score"></a>
</p>

<h1 align="center">glenv</h1>

<p align="center">
  <b>Sync .env files to GitLab CI/CD variables — bulk import, export, and manage environment variables via API</b><br>
  <i>Fast, concurrent CLI tool for GitLab secrets management and dotenv synchronization</i>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> •
  <a href="#features">Features</a> •
  <a href="#installation">Installation</a> •
  <a href="#usage">Usage</a> •
  <a href="#configuration">Configuration</a> •
  <a href="#faq">FAQ</a>
</p>

---

## What is glenv?

**glenv** is a command-line tool that synchronizes `.env` files with GitLab CI/CD variables. It solves the problem of managing GitLab environment variables at scale — bulk import, export, diff, and sync hundreds of variables in seconds using the GitLab API.

Instead of clicking through the GitLab web UI or writing fragile bash scripts, glenv provides:
- **Bulk operations** — import/export entire `.env` files with one command
- **Smart classification** — auto-detects masked, protected, and file-type variables
- **Safe workflow** — preview changes with diff before applying
- **Concurrent sync** — parallel API calls with built-in rate limiting
- **Multi-environment** — manage production, staging, and custom environments from config

### Why glenv over alternatives?

| Problem | glenv Solution |
|---------|----------------|
| GitLab UI is slow for many variables | Bulk sync hundreds of variables in seconds |
| Bash scripts are fragile and sequential | Concurrent workers with retry and rate limiting |
| No preview before changes | Diff command shows create/update/delete before sync |
| Manual variable classification | Auto-detects masked/protected/file from key patterns |
| Different configs per environment | Single YAML config for all environments |

**glenv** is written in Go — single static binary, no runtime dependencies, works on Linux, macOS, and Windows.

## Features

- **Concurrent sync** — configurable worker pool with token bucket rate limiter
- **Smart classification** — auto-detects masked, protected, and file-type variables from key patterns
- **Rate limit safe** — respects GitLab API limits, handles 429 with Retry-After, exponential backoff
- **Diff before sync** — preview changes before applying (create/update/delete)
- **Dry-run mode** — see what would happen without making any API calls
- **Multi-environment** — sync production, staging, or any custom environment from config
- **Export** — download current GitLab variables to `.env` file format
- **.env parser** — supports multiline values, quoted strings, comments, placeholder detection
- **Zero config** — works with just a token and project ID, config file is optional
- **Self-hosted support** — works with any GitLab instance, configurable rate limits

## Quick Start

```bash
# Install
go install github.com/ohmylock/glenv/cmd/glenv@latest

# Set credentials (or use config file)
export GITLAB_TOKEN="glpat-xxxxxxxxxxxx"
export GITLAB_PROJECT_ID="12345678"

# Sync .env file to GitLab
glenv sync -f .env.production -e production

# Preview changes first (recommended)
glenv diff -f .env.production -e production
```

## Installation

### Homebrew (macOS/Linux)

```bash
brew install ohmylock/tools/glenv
```

### Download binary

Download the appropriate binary for your platform from [releases](https://github.com/ohmylock/glenv/releases):

| Platform | Architecture | File |
|----------|--------------|------|
| macOS | Apple Silicon | `glenv_*_darwin_arm64.tar.gz` |
| macOS | Intel | `glenv_*_darwin_amd64.tar.gz` |
| Linux | x86_64 | `glenv_*_linux_amd64.tar.gz` |
| Linux | ARM64 | `glenv_*_linux_arm64.tar.gz` |
| Windows | x86_64 | `glenv_*_windows_amd64.zip` |

### Go install

```bash
go install github.com/ohmylock/glenv/cmd/glenv@latest
```

### Build from source

```bash
git clone https://github.com/ohmylock/glenv.git
cd glenv
make build
# binary: bin/glenv
```

## Usage

### Sync Variables

The primary command. Reads a `.env` file and syncs variables to GitLab:

```bash
# Sync single file to specific environment
glenv sync -f .env.production -e production

# Sync with dry-run (preview only)
glenv sync -f .env.staging -e staging --dry-run

# Sync all environments defined in config
glenv sync --all

# Override rate limits for self-hosted GitLab
glenv sync -f .env -e production --workers 10 --rate-limit 50
```

### Diff (Preview Changes)

Compare local `.env` file with current GitLab variables:

```bash
glenv diff -f .env.production -e production
```

Output:
```
+ DB_HOST=postgres.internal
~ API_KEY: *** → ***
- OLD_VAR
= LOG_LEVEL
```

### List Variables

```bash
# List all variables
glenv list

# Filter by environment
glenv list -e production
```

### Export Variables

Download GitLab variables to a local `.env` file:

```bash
glenv export -e production -o .env.production.backup
```

> **Note:** File-type variables (certificates, PEM keys) are excluded from the output and replaced with a comment `# KEY (file type, skipped)`. Use `glenv list` to see their presence.

### Delete Variables

```bash
# Delete specific variable
glenv delete -e production OLD_SECRET

# Delete multiple variables
glenv delete -e staging KEY1 KEY2 KEY3 --force
```

## Configuration

glenv works with zero config (just env vars), but a config file unlocks multi-environment workflows.

**Config file locations** (checked in order):
1. `--config` flag path
2. `.glenv.yml` in current directory
3. `~/.glenv.yml`

```yaml
# GitLab connection
gitlab:
  url: https://gitlab.com                     # self-hosted: https://gitlab.company.com
  token: ${GITLAB_TOKEN}                      # env var expansion supported
  project_id: "12345678"

# Rate limiting (safe defaults for gitlab.com)
rate_limit:
  requests_per_second: 10                     # max API requests/sec (gitlab.com allows ~33)
  max_concurrent: 5                           # parallel workers
  retry_max: 3                                # retries on failure
  retry_initial_backoff: 1s                   # backoff before first retry

# Environments
environments:
  production:
    file: deploy/gitlab-envs/.env.production
  staging:
    file: deploy/gitlab-envs/.env.staging

# Custom classification rules (extend built-in defaults)
classify:
  masked_patterns:                            # keys containing these → masked
    - "_TOKEN"
    - "SECRET"
    - "PASSWORD"
    - "API_KEY"
    - "DSN"
  masked_exclude:                             # exceptions (NOT masked)
    - "MAX_TOKENS"
    - "TIMEOUT"
    - "PORT"
  file_patterns:                              # keys containing these → file type
    - "PRIVATE_KEY"
    - "_CERT"
    - "_PEM"
  file_exclude:                               # exceptions (NOT file type)
    - "_PATH"
    - "_DIR"
    - "_URL"
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `GITLAB_TOKEN` | GitLab Personal Access Token (scope: `api`) |
| `GITLAB_PROJECT_ID` | Project ID or URL-encoded path |
| `GITLAB_URL` | GitLab instance URL (default: `https://gitlab.com`) |
| `NO_COLOR` | Disable colored output when set to any non-empty value (standard convention) |

Environment variables take precedence over config file values. CLI flags take precedence over everything.

## How It Works

### Sync Workflow

1. **Parse** — reads `.env` file, handles multiline values, skips placeholders
2. **Classify** — auto-detects masked/protected/file properties from key patterns and values
3. **Fetch** — gets current variables from GitLab (paginated, concurrent-safe)
4. **Diff** — calculates creates, updates, deletes, and unchanged
5. **Apply** — distributes changes across worker pool with rate limiting
6. **Report** — color-coded summary with timing and API call stats

### Variable Classification

glenv auto-detects variable properties:

| Property | Condition |
|----------|-----------|
| **masked** | Key matches secret pattern (`_TOKEN`, `SECRET`, `PASSWORD`, etc.) AND value is >= 8 characters AND value is single-line AND value contains only `[a-zA-Z0-9_:@-.+~=/]` characters |
| **protected** | Environment is `production` AND key matches secret pattern |
| **file type** | Key matches file pattern (`PRIVATE_KEY`, `_CERT`, `_PEM`) OR value contains PEM headers (`-----BEGIN`) |

Variables with placeholder values (`your_`, `CHANGE_ME`, `REPLACE_WITH_`) are skipped.
Variables with interpolation (`${VAR}`) are skipped.

### Rate Limiting

glenv uses a token bucket rate limiter to stay within GitLab API limits:

| GitLab Instance | Limit | Default Config |
|----------------|-------|----------------|
| gitlab.com | 2,000 req/min (~33/sec) | 10 req/sec, 5 workers |
| Self-hosted | Configurable | Adjust via config |

On 429 responses:
1. Parse `Retry-After` header
2. Wait the specified duration
3. Retry with exponential backoff + jitter
4. Max 3 retries per operation

### .env File Format

Supported syntax:

```bash
# Comments are skipped
KEY=value
QUOTED="value with spaces"
SINGLE_QUOTED='value'

# Multiline values
PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA...
-----END RSA PRIVATE KEY-----"

# These are skipped automatically:
PLACEHOLDER=your_api_key_here       # placeholder detected
INTERPOLATED=${OTHER_VAR}/path      # interpolation detected
```

## Options Reference

### Global Options

| Flag | Short | Env Var | Description | Default |
|------|-------|---------|-------------|---------|
| `--config` | `-c` | | Config file path | `.glenv.yml` |
| `--token` | | `GITLAB_TOKEN` | GitLab access token | |
| `--project` | | `GITLAB_PROJECT_ID` | Project ID | |
| `--url` | | `GITLAB_URL` | GitLab URL | `https://gitlab.com` |
| `--dry-run` | `-n` | | Preview mode | `false` |
| `--no-color` | | `NO_COLOR` | Disable colors | `false` |
| `--workers` | `-w` | | Concurrent workers | `5` |
| `--rate-limit` | | | Max requests/sec | `10` |

### Sync Options

| Flag | Short | Description |
|------|-------|-------------|
| `--file` | `-f` | Path to .env file |
| `--environment` | `-e` | GitLab environment scope |
| `--all` | `-a` | Sync all environments from config |
| `--delete-missing` | | Delete variables not in .env file |
| `--no-auto-classify` | | Disable smart classification |
| `--force` | | Skip confirmation prompts |

### Export Options

| Flag | Short | Description |
|------|-------|-------------|
| `--environment` | `-e` | Environment to export |
| `--output` | `-o` | Output file path (default: stdout) |

## Examples

### Basic: Single Environment

```bash
export GITLAB_TOKEN="glpat-xxxxxxxxxxxx"
export GITLAB_PROJECT_ID="12345678"

# Preview what will change
glenv diff -f .env.production -e production

# Apply changes
glenv sync -f .env.production -e production
```

### Config: Multiple Environments

`.glenv.yml`:
```yaml
gitlab:
  token: ${GITLAB_TOKEN}
  project_id: "12345678"

environments:
  staging:
    file: deploy/.env.staging
  production:
    file: deploy/.env.production
    protected: true
```

```bash
# Sync all environments
glenv sync --all

# Sync specific environment
glenv sync -e production
```

### Self-Hosted GitLab with Higher Limits

```yaml
gitlab:
  url: https://gitlab.company.com
  token: ${GITLAB_TOKEN}
  project_id: "42"

rate_limit:
  requests_per_second: 50
  max_concurrent: 10
```

### CI/CD Pipeline Integration

```yaml
# .gitlab-ci.yml
sync-variables:
  image: golang:1.23-alpine
  script:
    - go install github.com/ohmylock/glenv/cmd/glenv@latest
    - glenv sync -f deploy/.env.${CI_ENVIRONMENT_NAME} -e ${CI_ENVIRONMENT_NAME}
  variables:
    GITLAB_TOKEN: ${DEPLOY_TOKEN}
    GITLAB_PROJECT_ID: ${CI_PROJECT_ID}
```

## Releasing

glenv uses [goreleaser](https://goreleaser.com/) for cross-platform releases.

### One-Command Release

```bash
# Tag and release
git tag v1.0.0
git push origin v1.0.0
# GitHub Actions handles the rest

# Or release locally
make release
```

### Makefile Targets

```bash
make build          # compile binary to bin/glenv
make test           # run tests with race detector and coverage
make lint           # run golangci-lint (fallback: go vet)
make install        # install to /usr/local/bin
make release        # goreleaser release
make release-check  # goreleaser dry-run (no publish)
make clean          # remove build artifacts
```

## Development

### Requirements

- Go 1.25+
- golangci-lint (for `make lint`)
- goreleaser (for `make release`)

### Running Tests

```bash
make test           # unit tests with coverage
make lint           # static analysis
go test -race ./... # race detector
```

### Integration Tests

Set `GITLAB_TOKEN` and `GITLAB_TEST_PROJECT_ID` to run tests against a real GitLab instance:

```bash
GITLAB_TOKEN=glpat-xxx GITLAB_TEST_PROJECT_ID=123 go test -tags=integration ./...
```

## FAQ

<details>
<summary><b>Is it safe to run with multiple workers?</b></summary>

Yes. glenv uses a token bucket rate limiter that controls the overall API request rate regardless of worker count. Workers share the rate limiter, so increasing workers doesn't increase the API request rate — it just distributes the work more efficiently. The default of 10 req/sec is well under gitlab.com's 2,000 req/min limit.
</details>

<details>
<summary><b>What happens if GitLab rate-limits me?</b></summary>

glenv handles 429 responses automatically. It reads the `Retry-After` header, waits the specified duration, then retries with exponential backoff. After 3 failed retries (configurable), the operation is marked as failed and reported in the summary.
</details>

<details>
<summary><b>Does it work with self-hosted GitLab?</b></summary>

Yes. Set `gitlab.url` in your config file or use `--url` flag. Self-hosted instances may have different (or no) rate limits — adjust `rate_limit.requests_per_second` accordingly.
</details>

<details>
<summary><b>How does it handle masked variables?</b></summary>

GitLab requires masked variables to be at least 8 characters and single-line. glenv auto-detects which variables should be masked based on key name patterns (TOKEN, SECRET, PASSWORD, etc.) and validates the value meets GitLab's requirements. You can customize patterns via config.
</details>

<details>
<summary><b>Can I sync multiple projects?</b></summary>

Currently glenv syncs one project per invocation. For multiple projects, use separate config files:
```bash
glenv sync --config project-a.yml --all
glenv sync --config project-b.yml --all
```
</details>

<details>
<summary><b>What .env formats are supported?</b></summary>

Standard `.env` format: `KEY=VALUE`, with support for single/double quotes, multiline quoted values, and comments. Placeholders (`your_`, `CHANGE_ME`) and variable interpolation (`${VAR}`) are detected and skipped. See [.env File Format](#env-file-format) for details.
</details>

## Alternatives

| Tool | Language | Features |
|------|----------|----------|
| **glenv** | Go | Concurrent sync, auto-classification, rate limiting, diff preview |
| [GlabEnv](https://github.com/arisnacg/nodejs-glabenv) | Node.js | Basic sync/export |
| [gitlab-dotenv](https://github.com/apicore-engineering/gitlab-dotenv) | Python | Variable management |
| [glab variable](https://docs.gitlab.com/cli/variable/) | Go | Official CLI (single variable ops) |

## Contributing

Contributions are welcome! Please read the [Contributing Guide](CONTRIBUTING.md) before submitting a PR.

## License

MIT License — see [LICENSE](LICENSE) file.

---

<p align="center">
  Made with ❤️ for the DevOps community
</p>

<!--
SEO Keywords (primary):
- gitlab ci cd variables sync tool
- gitlab environment variables cli
- sync env file to gitlab
- dotenv gitlab synchronization
- gitlab variables bulk import export
- gitlab secrets management cli
- gitlab api variables automation

SEO Keywords (secondary):
- cicd configuration management
- devops secrets automation tool
- gitlab self-hosted variables sync
- manage gitlab env variables programmatically
- gitlab variable migration tool
- export gitlab variables to env file
- import env file to gitlab ci cd
- gitlab ci variables bulk update upload
- gitlab project variables cli
- dotenv to gitlab ci cd
- gitlab variable manager golang
- alternative to glab variable
- gitlab environment scope variables
- masked protected variables gitlab
- concurrent gitlab api client

Related searches:
- how to bulk upload variables to gitlab
- sync local env with gitlab ci cd
- gitlab variables import from file
- automate gitlab environment variables
- gitlab dotenv integration
- manage gitlab secrets from command line
-->
