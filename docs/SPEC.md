# glenv - Technical Specification

## Overview

**glenv** is a fast, concurrent CLI tool written in Go for managing GitLab CI/CD variables.
It reads `.env` files and syncs them to GitLab projects via the CI/CD Variables API, with smart
auto-classification of variable types (masked, protected, file), rate limiting, and multi-environment support.

## Problem Statement

Managing GitLab CI/CD variables manually through the UI is tedious and error-prone, especially when:
- A project has 50+ variables across multiple environments (staging, production)
- Variables need to be synced between local `.env` files and GitLab
- Some variables must be masked, protected, or stored as files (SSH keys, certificates)
- Multiple team members need reproducible variable setup

Existing bash scripts work but are sequential (slow for large variable sets) and fragile.

## Goals

1. **Fast** - concurrent uploads with configurable worker pool
2. **Safe** - rate limiting to avoid 429 bans, dry-run mode, diff before sync
3. **Smart** - auto-detect variable types (masked, protected, file) from key patterns and values
4. **Simple** - single binary, minimal config, works out of the box
5. **Publishable** - ready for open-source release on GitHub with proper docs and CI/CD

## GitLab API Rate Limits

### gitlab.com (SaaS)
- **2,000 requests/min** for authenticated API calls (~33 req/sec)
- 429 response with `Retry-After` header when exceeded
- Rate limit headers: `RateLimit-Limit`, `RateLimit-Remaining`, `RateLimit-Reset`

### Self-hosted
- **Configurable** (disabled by default)
- Default when enabled: 7,200 requests/hour
- Admin can set custom per-user/per-IP limits

### Our strategy
- Default: **10 requests/sec** with **5 concurrent workers** (conservative, safe for any GitLab)
- Token bucket rate limiter (`golang.org/x/time/rate`)
- Respect `Retry-After` header on 429 responses
- Exponential backoff with jitter on retries (max 3 retries)
- Configurable via config file for self-hosted with higher limits

## Architecture

### Project Structure

```
glenv/
├── cmd/glenv/
│   └── main.go                 # CLI entry, go-flags, subcommands
├── pkg/
│   ├── config/                 # YAML config loading + defaults
│   │   ├── config.go
│   │   └── config_test.go
│   ├── gitlab/                 # GitLab API client with rate limiting
│   │   ├── client.go           # HTTP client, auth, rate limiting
│   │   ├── client_test.go
│   │   ├── variables.go        # CRUD operations for variables
│   │   └── variables_test.go
│   ├── envfile/                # .env file parser
│   │   ├── parser.go           # Parse with multiline, quotes, comments
│   │   └── parser_test.go
│   ├── classifier/             # Variable type classification
│   │   ├── classifier.go       # masked/protected/file detection
│   │   └── classifier_test.go
│   └── sync/                   # Sync engine
│       ├── engine.go           # Diff calculation, concurrent sync
│       └── engine_test.go
├── docs/
│   ├── SPEC.md                 # This file
│   ├── configuration.md        # Config file reference
│   └── examples.md             # Usage examples
├── .goreleaser.yml             # Release automation
├── .github/
│   └── workflows/
│       ├── ci.yml              # Test + lint on PR
│       └── release.yml         # Build + publish on tag
├── Makefile                    # Build, test, lint, release
├── go.mod
├── README.md                   # Main documentation
├── LICENSE                     # MIT
└── .gitignore
```

### Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/jessevdk/go-flags` | CLI parsing with subcommands |
| `gopkg.in/yaml.v3` | YAML config file parsing |
| `golang.org/x/time/rate` | Token bucket rate limiter |
| `github.com/fatih/color` | Colored terminal output |
| `github.com/stretchr/testify` | Test assertions |

No heavy frameworks. Minimal, focused dependencies.

## CLI Interface

### Commands

```bash
# Sync variables from .env file to GitLab (default command)
glenv sync -f .env.production -e production

# Sync all environments defined in config
glenv sync --all

# Show diff between local .env and GitLab (no changes made)
glenv diff -f .env.production -e production

# Export GitLab variables to .env file
glenv export -e production -o .env.production.backup

# List all variables in GitLab project
glenv list [-e production]

# Delete variables (with confirmation)
glenv delete -e production --key DB_PASSWORD

# Show version
glenv version
```

### Global Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--config` | `-c` | Config file path | `.glenv.yml` |
| `--token` | | GitLab token (overrides config) | `$GITLAB_TOKEN` |
| `--project` | | Project ID (overrides config) | `$GITLAB_PROJECT_ID` |
| `--url` | | GitLab URL (overrides config) | `https://gitlab.com` |
| `--dry-run` | `-n` | Show what would happen, don't apply | `false` |
| `--debug` | `-d` | Enable debug logging | `false` |
| `--no-color` | | Disable colored output | `false` |
| `--workers` | `-w` | Number of concurrent workers | `5` |
| `--rate-limit` | | Max requests per second | `10` |

### Sync Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--file` | `-f` | Path to .env file |
| `--environment` | `-e` | GitLab environment scope |
| `--all` | `-a` | Sync all environments from config |
| `--delete-missing` | | Delete GitLab vars not in .env file |
| `--no-auto-classify` | | Disable auto-detection of masked/protected/file |
| `--force` | | Skip confirmation for destructive operations |

## Config File

File: `.glenv.yml` (project root) or `~/.glenv.yml` (global).
Project config takes priority over global.

```yaml
# GitLab connection
gitlab:
  url: https://gitlab.com                    # or self-hosted URL
  token: ${GITLAB_TOKEN}                     # supports env var expansion
  project_id: "12345678"                     # or URL-encoded path: "group%2Fproject"

# Rate limiting (safe defaults for gitlab.com)
rate_limit:
  requests_per_second: 10                    # max API calls/sec
  max_concurrent: 5                          # worker pool size
  retry_max: 3                               # max retries on failure
  retry_initial_backoff: 1s                  # first retry delay

# Environment definitions
environments:
  production:
    file: deploy/gitlab-envs/.env.production
    protected: true                          # mark all vars as protected
  staging:
    file: deploy/gitlab-envs/.env.staging
    protected: false

# Variable classification rules (extends built-in defaults)
classify:
  # Additional patterns for masked variables (secrets)
  masked_patterns:
    - "_TOKEN"
    - "SECRET"
    - "PASSWORD"
    - "API_KEY"
    - "DSN"
  # Patterns that should NOT be masked
  masked_exclude:
    - "MAX_TOKENS"
    - "TIMEOUT"
    - "PORT"
  # Patterns for file-type variables
  file_patterns:
    - "PRIVATE_KEY"
    - "_CERT"
    - "_PEM"
  # Patterns excluded from file type
  file_exclude:
    - "_PATH"
    - "_DIR"
    - "_URL"
```

## Core Features

### 1. .env File Parser

Supports:
- Standard `KEY=VALUE` format
- Single and double quoted values
- **Multiline values** (quoted strings spanning multiple lines)
- Comments (`#`) and blank lines
- Skip placeholders (`your_`, `CHANGE_ME`, `REPLACE_WITH_`)
- Skip variable interpolation (`${VAR}`)

Does not support:
- Shell command substitution (`$(command)`)
- Inline comments (entire line after `#` is kept as value)

### 2. Variable Classification

Auto-detects variable properties based on key name patterns and value content:

| Property | Detection Logic |
|----------|----------------|
| **masked** | Key matches secret patterns AND value >= 8 chars AND single-line |
| **protected** | Environment is production AND key matches secret patterns |
| **file type** | Key matches file patterns OR value contains PEM headers |

Built-in patterns are derived from the battle-tested bash script.
Custom patterns can be added via config file.

### 3. Concurrent Sync Engine

```
                    ┌─────────────┐
                    │  .env file  │
                    └──────┬──────┘
                           │ parse
                    ┌──────▼──────┐
                    │  Variables  │
                    └──────┬──────┘
                           │ classify
                    ┌──────▼──────┐
          ┌────────│   Diff Engine │────────┐
          │        └──────────────┘        │
          │ fetch current                   │ calculate
          │ from GitLab                     │ changes
          │                                 │
   ┌──────▼──────┐                  ┌──────▼──────┐
   │  GitLab API  │                  │   Changes   │
   │  (list vars) │                  │ create/upd/ │
   └─────────────┘                  │   delete    │
                                    └──────┬──────┘
                                           │
                              ┌─────────────┼─────────────┐
                              │             │             │
                        ┌─────▼─────┐ ┌─────▼─────┐ ┌─────▼─────┐
                        │ Worker 1  │ │ Worker 2  │ │ Worker N  │
                        └─────┬─────┘ └─────┬─────┘ └─────┬─────┘
                              │             │             │
                        ┌─────▼─────────────▼─────────────▼─────┐
                        │        Rate Limiter (token bucket)     │
                        └─────────────────┬─────────────────────┘
                                          │
                                   ┌──────▼──────┐
                                   │  GitLab API  │
                                   └─────────────┘
```

Workflow:
1. Parse `.env` file into key-value pairs
2. Classify each variable (masked, protected, file)
3. Fetch current variables from GitLab (paginated)
4. Calculate diff: create / update / unchanged / delete
5. Display diff (or apply in sync mode)
6. Distribute changes across worker pool
7. Each worker respects rate limiter before API call
8. Retry with backoff on failures, respect 429 Retry-After

### 4. Rate Limiter

- **Token bucket** algorithm via `golang.org/x/time/rate`
- Configurable rate (default: 10/sec) and burst (default: equals rate)
- On 429 response: parse `Retry-After` header, sleep, then retry
- Exponential backoff with jitter: `backoff * 2^attempt + random(0, 500ms)`
- Max 3 retries per operation (configurable)

### 5. Output

Color-coded terminal output:
```
glenv v1.0.0

Syncing: .env.production → project 12345678 (production)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ✓ Created:   DB_HOST
  ✓ Created:   DB_PASSWORD                    [masked] [protected]
  ↻ Updated:   API_KEY                        [masked]
  = Unchanged: LOG_LEVEL
  ⊘ Skipped:   PLACEHOLDER_VAR               (placeholder)
  ✗ Failed:    BROKEN_VAR                     (HTTP 400: value too long)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Created: 2 | Updated: 1 | Unchanged: 1 | Skipped: 1 | Failed: 1
  Duration: 1.2s | API calls: 5 | Rate: 4.2 req/s
```

### 6. Diff Mode

Shows what would change without applying:
```
glenv diff -f .env.production -e production

  + DB_HOST = "postgres.internal"              (create)
  ~ API_KEY = "sk-***" → "sk-***"             (update, masked)
  - OLD_VAR                                    (delete, not in .env)
  = LOG_LEVEL = "INFO"                         (unchanged)
```

### 7. Export

Export existing GitLab variables to `.env` format:
```bash
glenv export -e production -o .env.production.backup
```

## Release Automation

### goreleaser

`.goreleaser.yml` configuration for:
- Cross-compilation: linux/darwin/windows (amd64/arm64)
- Binary naming: `glenv_OS_ARCH`
- Checksums and signatures
- GitHub release with changelog
- Homebrew tap formula

### Makefile targets

```makefile
make build          # compile binary
make test           # run tests with coverage
make lint           # golangci-lint
make release        # goreleaser release (requires GITHUB_TOKEN)
make release-check  # goreleaser dry-run
make install        # install to /usr/local/bin
```

### Release workflow

```bash
# 1. Tag the release
git tag v1.0.0
git push origin v1.0.0

# 2. GitHub Actions triggers goreleaser automatically
# OR locally:
make release
```

## API Endpoints Used

| Operation | Method | Endpoint |
|-----------|--------|----------|
| List variables | GET | `/projects/:id/variables` |
| Get variable | GET | `/projects/:id/variables/:key` |
| Create variable | POST | `/projects/:id/variables` |
| Update variable | PUT | `/projects/:id/variables/:key` |
| Delete variable | DELETE | `/projects/:id/variables/:key` |

All endpoints support `filter[environment_scope]` for scoped operations.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| 429 Too Many Requests | Parse Retry-After, wait, retry |
| 401 Unauthorized | Stop immediately, clear error message |
| 404 Not Found | Skip (variable doesn't exist on update) |
| Network error | Retry with backoff |
| Invalid .env format | Log warning, skip line, continue |
| Config not found | Use defaults + env vars |
| Ctrl+C | Graceful shutdown, report partial progress |

## Testing Strategy

- **Unit tests**: Each package has `_test.go` files
- **Integration tests** (optional, with `GITLAB_TOKEN`): Real API calls against test project
- **Table-driven tests**: For parser and classifier
- **Mock HTTP server**: For GitLab client tests
- **Race detector**: `go test -race ./...`

## Non-Goals

- Group-level variables (project-level only for v1)
- Variable history/versioning
- Web UI
- Variable value generation
- Encryption at rest
