# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-02-23

### Added

- **Core CLI commands:**
  - `sync` — push variables from .env file to GitLab CI/CD
  - `diff` — preview changes before applying (create/update/delete)
  - `list` — list all GitLab CI/CD variables with filtering
  - `export` — download GitLab variables to .env file format
  - `delete` — remove one or more variables

- **Smart variable classification:**
  - Auto-detect masked variables (tokens, secrets, passwords)
  - Auto-detect protected variables for production
  - Auto-detect file-type variables (certificates, PEM keys)
  - Customizable patterns via config file

- **Concurrent sync engine:**
  - Configurable worker pool for parallel API calls
  - Token bucket rate limiter (safe defaults for gitlab.com)
  - Automatic retry with exponential backoff + jitter
  - Respects `Retry-After` header on 429 responses

- **Multi-environment support:**
  - Single YAML config for all environments
  - Environment variable expansion in config (`${GITLAB_TOKEN}`)
  - `--all` flag to sync all configured environments

- **.env parser:**
  - Multiline quoted values (for certificates, SSH keys)
  - Single and double quoted strings
  - Comments and empty lines
  - Placeholder detection (skips `your_`, `CHANGE_ME`, etc.)
  - Interpolation detection (skips `${VAR}` references)

- **CLI features:**
  - Dry-run mode (`--dry-run`)
  - Colored output (respects `NO_COLOR`)
  - Environment scope filtering (`-e production`)
  - Self-hosted GitLab support (`--url`)

### Security

- Tokens are never logged or displayed
- Masked variable values shown as `***` in diff output
- `.glenv.yml` excluded from git by default

[0.1.0]: https://github.com/ohmylock/glenv/releases/tag/v0.1.0
