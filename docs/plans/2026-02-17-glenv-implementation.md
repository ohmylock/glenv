# glenv — Full Implementation Plan

## Overview

Implement the complete glenv CLI tool from scratch: a fast, concurrent Go CLI for managing GitLab CI/CD variables via `.env` files. The tool reads `.env` files, auto-classifies variables (masked/protected/file), and syncs them to GitLab projects with rate limiting and concurrent worker pool.

Build order is strictly bottom-up: foundation packages first, CLI composition last, release infrastructure at the end.

## Context

- **Files involved:** All new — project is 0% implemented (only docs exist)
- **Existing patterns:** Technical spec at `docs/SPEC.md`, design doc at `docs/plans/2026-02-17-glenv-design.md`
- **Module:** `github.com/ohmylock/glenv`
- **Go version:** 1.25
- **Dependencies:** `github.com/jessevdk/go-flags`, `gopkg.in/yaml.v3`, `golang.org/x/time/rate`, `github.com/fatih/color`, `github.com/stretchr/testify`

## Development Approach

- Testing approach: **TDD** — write tests first, then implementation
- Complete each task fully before moving to the next
- Every task includes writing tests before code
- `go test -race ./...` must pass before starting the next task

---

## Implementation Tasks

### Task 1: Project Foundation

---
model: haiku
priority: P0
complexity: Simple
---

**Description:** Initialize Go module, create directory structure, build tooling, and project scaffolding.

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `.gitignore`
- Create: `LICENSE` (MIT, Copyright 2026 ohmylock)
- Create: stub `doc.go` files in each package directory

**Steps:**
- [x] Create directory structure: `cmd/glenv/`, `pkg/{config,gitlab,envfile,classifier,sync}/`, `.github/workflows/`
- [x] Create `go.mod` with module `github.com/ohmylock/glenv`, Go 1.25, and all dependencies
- [x] Run `go mod tidy` to generate `go.sum`
- [x] Create `Makefile` with targets: build, test, lint, install, release, release-check, clean. Use LDFLAGS with `-X main.version=$(VERSION)`
- [x] Create `.gitignore` excluding: `.bin/`, `dist/`, `coverage.out`, `*.env`, `.glenv.yml`
- [x] Create `LICENSE` (MIT)
- [x] Create stub `doc.go` in each package with `package <name>` declaration
- [x] Verify: `go build ./...` compiles without errors

---

### Task 2: .env Parser (TDD)

---
model: sonnet
priority: P0
complexity: Medium
---

**Description:** Implement `.env` file parser with support for KEY=VALUE, quoted values, multiline, comments, and placeholder/interpolation skip.

**Files:**
- Create: `pkg/envfile/parser_test.go`
- Create: `pkg/envfile/parser.go`

**Types to implement:**
- `Variable{Key, Value, Line string/int}`
- `ParseResult{Variables []Variable, Skipped []SkippedLine}`
- `SkipReason` constants: `SkipPlaceholder`, `SkipInterpolation`, `SkipComment`, `SkipBlank`
- `SkippedLine{Line int, Key string, Reason SkipReason}`

**Public API:** `ParseFile(path string) (*ParseResult, error)`, `ParseReader(r io.Reader) (*ParseResult, error)`

**Steps:**
- [x] Write table-driven tests for all cases:
  - Simple `KEY=VALUE` (unquoted)
  - Double-quoted: `KEY="value with spaces"`
  - Single-quoted: `KEY='value'`
  - Empty value: `KEY=`
  - Comment line: `# comment`
  - Blank line
  - Placeholder skip: `your_`, `CHANGE_ME`, `REPLACE_WITH_`
  - Interpolation skip: `${VAR}`
  - Multiline (PEM block in double quotes spanning lines)
  - Value containing `#` (not inline comment — kept as value)
  - Full file integration test with temp file
- [x] Implement `ParseReader()` with `bufio.Scanner`:
  - Skip blank lines and `#` comments
  - Split on first `=`
  - Detect opening quote, enter multiline mode if unbalanced
  - Strip outer quotes
  - Check placeholder patterns (case-insensitive)
  - Check interpolation (`${`)
- [x] Implement `ParseFile()` as wrapper opening file and calling `ParseReader()`
- [x] Run `go test -race ./pkg/envfile/` — all tests pass

---

### Task 3: Variable Classifier (TDD)

---
model: sonnet
priority: P0
complexity: Medium
---

**Description:** Implement auto-classification of variables as masked, protected, or file-type based on key patterns and value content.

**Files:**
- Create: `pkg/classifier/classifier_test.go`
- Create: `pkg/classifier/classifier.go`

**Types to implement:**
- `Classification{Masked bool, Protected bool, VarType string}`
- `Rules{MaskedPatterns, MaskedExclude, FilePatterns, FileExclude []string}`
- `Classifier` struct with merged rules

**Built-in patterns:**
- Masked: `_TOKEN`, `SECRET`, `PASSWORD`, `API_KEY`, `DSN`
- Masked exclude: `MAX_TOKENS`, `TIMEOUT`, `PORT`
- File: `PRIVATE_KEY`, `_CERT`, `_PEM`
- File exclude: `_PATH`, `_DIR`, `_URL`
- PEM detection: `-----BEGIN` in value

**Classification logic:**
- masked = key matches secret patterns AND value >= 8 chars AND single-line (no `\n`)
- protected = environment == "production" AND key matches secret patterns
- file = key matches file patterns OR value contains PEM header

**Steps:**
- [x] Write table-driven tests:
  - `API_KEY` + long value → masked=true
  - `API_KEY` + short value → masked=false
  - `MAX_TOKENS` + long value → masked=false (excluded)
  - `DB_PASSWORD` + production → masked=true, protected=true
  - `LOG_LEVEL` + production → masked=false, protected=false
  - `PRIVATE_KEY` + any value → type=file
  - `CA_CERT` + any value → type=file
  - `CERT_PATH` → type=env_var (excluded from file)
  - Value with `-----BEGIN` → type=file
  - Multiline value for TOKEN → masked=false
  - Custom user rules merged with built-in
- [x] Implement `New(userRules Rules) *Classifier` with rule merging
- [x] Implement `Classify(key, value, environment string) Classification`
- [x] Implement `matchesMasked(key)` and `matchesFile(key, value)` with exclude-first logic
- [x] Run `go test -race ./pkg/classifier/` — all tests pass

---

### Task 4: YAML Config (TDD)

---
model: sonnet
priority: P0
complexity: Medium
---

**Description:** Implement YAML config loading with defaults, env var expansion, and priority chain.

**Files:**
- Create: `pkg/config/config_test.go`
- Create: `pkg/config/config.go`

**Types to implement:**
- `Config{GitLab, RateLimit, Environments, Classify}`
- `GitLabConfig{URL, Token, ProjectID string}`
- `RateLimitConfig{RequestsPerSecond float64, MaxConcurrent int, RetryMax int, RetryInitialBackoff time.Duration}`
- `EnvironmentConfig{File string, Protected bool}`

**Defaults:** URL=`https://gitlab.com`, rate=10/s, workers=5, retries=3, backoff=1s

**Priority:** CLI flags > config file > env vars > defaults (Load applies defaults→env→YAML; CLI flags applied by caller)

**Config search:** `--config` flag → `.glenv.yml` → `~/.glenv.yml`

**Env vars:** `GITLAB_TOKEN`, `GITLAB_PROJECT_ID`, `GITLAB_URL`

**Steps:**
- [x] Write tests:
  - `TestLoad_Defaults` — no config, no env → verify defaults
  - `TestLoad_EnvVars` — set env vars → verify override
  - `TestLoad_ConfigFile` — temp YAML file → verify loaded
  - `TestLoad_EnvExpansion` — `token: ${GITLAB_TOKEN}` → verify expansion
  - `TestLoad_MissingConfigPath` — explicit bad path → error
  - `TestValidate_MissingToken` → error
  - `TestValidate_MissingProject` → error
  - `TestResolveConfigPath` — local `.glenv.yml` found first
- [x] Implement `defaults() Config`
- [x] Implement `applyEnvVars(cfg)` — overlay from `GITLAB_*` env vars
- [x] Implement `resolveConfigPath(override string)` — search chain
- [x] Implement `expandEnvVars(cfg)` — `os.ExpandEnv` on string fields
- [x] Implement `Load(configPath string) (*Config, error)` — defaults → env → YAML → expand
- [x] Implement `Validate() error` — require token and project_id
- [x] Run `go test -race ./pkg/config/` — all tests pass

---

### Task 5: GitLab HTTP Client (TDD)

---
model: opus
priority: P0
complexity: Large
---

**Description:** Implement HTTP client with token bucket rate limiting, exponential backoff with jitter, retry logic, and 429/Retry-After handling.

**Files:**
- Create: `pkg/gitlab/client_test.go`
- Create: `pkg/gitlab/client.go`

**Types:**
- `ClientConfig{BaseURL, Token string, RequestsPerSecond float64, Burst int, RetryMax int, RetryInitialBackoff time.Duration, HTTPClient *http.Client}`
- `Client{cfg, limiter *rate.Limiter, http *http.Client}`

**Key implementation details:**
- Token bucket: `rate.NewLimiter(rate.Limit(rps), burst)` — `Wait(ctx)` before each request
- Auth: `PRIVATE-TOKEN` header
- Retry: `base * 2^attempt + random(0, 500ms)`, max 3 attempts
- 429: parse `Retry-After` header, sleep, retry
- 401: immediate stop, no retry
- Request body cloning for retries (buffer body before first attempt)

**Steps:**
- [x] Write mock HTTP server test helper: `setupMockServer(t, handler) (*httptest.Server, *Client)`
- [x] Write tests:
  - `TestDo_AuthHeader` — verify PRIVATE-TOKEN header sent
  - `TestDo_RateLimiting` — verify rate limiter is called
  - `TestDo_Retry_NetworkError` — mock drops connection → 3 retries
  - `TestDo_Retry_429_RetryAfter` — mock returns 429 + Retry-After → wait and retry
  - `TestDo_401_NoRetry` — mock returns 401 → immediate error, no retry
  - `TestDo_Success` — 200 response → returned correctly
- [x] Implement `NewClient(cfg ClientConfig) *Client`
- [x] Implement `do(ctx, req) (*http.Response, error)` — core method with limiter, retry, backoff
- [x] Implement `backoff(attempt int, extra time.Duration) time.Duration`
- [x] Implement `parseRetryAfter(resp) time.Duration`
- [x] Implement `cloneRequest(req) (*http.Request, error)` for retry body replay
- [x] Run `go test -race ./pkg/gitlab/` — all tests pass

---

### Task 6: GitLab Variables API (TDD)

---
model: sonnet
priority: P0
complexity: Medium
---

**Description:** Implement CRUD operations for GitLab CI/CD variables with pagination and environment scope filtering.

**Files:**
- Create: `pkg/gitlab/variables_test.go`
- Create: `pkg/gitlab/variables.go`

**Types:**
- `Variable{Key, Value, VariableType, EnvironmentScope string, Protected, Masked, Raw bool}`
- `CreateRequest{Key, Value, VariableType, EnvironmentScope string, Protected, Masked bool}`
- `ListOptions{EnvironmentScope string, Page, PerPage int}`

**API endpoints:**
- GET `/projects/:id/variables` (paginated, `per_page=100`)
- GET `/projects/:id/variables/:key`
- POST `/projects/:id/variables`
- PUT `/projects/:id/variables/:key`
- DELETE `/projects/:id/variables/:key`
- All support `filter[environment_scope]`

**Steps:**
- [x] Write tests with mock server:
  - `TestListVariables_SinglePage` — 3 vars, no X-Next-Page
  - `TestListVariables_MultiPage` — page 1 has X-Next-Page, page 2 empty
  - `TestListVariables_EnvScope` — verify filter query param
  - `TestCreateVariable` — POST body matches request, response parsed
  - `TestUpdateVariable` — PUT to correct URL
  - `TestDeleteVariable` — DELETE returns 204
  - `TestGetVariable_NotFound` — 404 returns nil, nil
- [x] Implement `ListVariables(ctx, projectID, opts) ([]Variable, error)` with pagination loop
- [x] Implement `GetVariable(ctx, projectID, key, envScope) (*Variable, error)`
- [x] Implement `CreateVariable(ctx, projectID, req) (*Variable, error)`
- [x] Implement `UpdateVariable(ctx, projectID, req) (*Variable, error)`
- [x] Implement `DeleteVariable(ctx, projectID, key, envScope) error`
- [x] Run `go test -race ./pkg/gitlab/` — all tests pass

---

### Task 7: Sync Engine (TDD)

---
model: opus
priority: P0
complexity: Large
---

**Description:** Implement diff calculation and concurrent worker pool for applying changes to GitLab.

**Files:**
- Create: `pkg/sync/engine_test.go`
- Create: `pkg/sync/engine.go`

**Types:**
- `ChangeKind` constants: `ChangeCreate`, `ChangeUpdate`, `ChangeDelete`, `ChangeUnchanged`, `ChangeSkipped`
- `Change{Kind, Key, OldValue, NewValue string, Classification, SkipReason string}`
- `DiffResult{Changes []Change}`
- `Result{Change, Error}`
- `SyncReport{Created, Updated, Deleted, Unchanged, Skipped, Failed int, Duration, APICalls int, Errors []error}`
- `Engine{client, classifier, opts, projectID}`, `Options{Workers, DryRun, DeleteMissing bool, Environment string}`

**Key architecture:**
- Worker pool: N goroutines read from `taskCh chan Change`, results to `resultCh chan Result`
- Graceful shutdown: ctx cancellation → limiter.Wait returns → workers stop → partial report
- `ApplyWithCallback(ctx, diff, cb func(Result)) SyncReport` for streaming output

**Steps:**
- [ ] Write tests:
  - `TestDiff_CreateNew` — local var not in remote → ChangeCreate
  - `TestDiff_UpdateChanged` — same key, different value → ChangeUpdate
  - `TestDiff_Unchanged` — same key, same value → ChangeUnchanged
  - `TestDiff_DeleteMissing_Enabled` — remote not in local → ChangeDelete
  - `TestDiff_DeleteMissing_Disabled` — no delete
  - `TestApply_DryRun` — no API calls
  - `TestApply_Concurrent` — 20 tasks, 5 workers, no race (with `-race`)
  - `TestApply_ContextCancel` — cancel mid-apply → partial report
  - `TestApply_ErrorHandling` — one API fails → Failed=1, others succeed
- [ ] Implement `NewEngine(client, classifier, opts, projectID)`
- [ ] Implement `Diff(ctx, local []envfile.Variable, remote []gitlab.Variable, envScope string) DiffResult`
- [ ] Implement `Apply(ctx, diff) SyncReport` with worker pool
- [ ] Implement `ApplyWithCallback(ctx, diff, cb func(Result)) SyncReport`
- [ ] Implement `applyOne(ctx, task Change) Result` — routes to Create/Update/Delete
- [ ] Run `go test -race ./pkg/sync/` — all tests pass

---

### Task 8: CLI Commands

---
model: opus
priority: P0
complexity: Large
---

**Description:** Implement CLI entry point with go-flags subcommands, all 6 commands, colored output, and graceful Ctrl+C.

**Files:**
- Create: `cmd/glenv/main.go`

**Commands:** sync, diff, list, export, delete, version

**Global flags:** `--config`, `--token`, `--project`, `--url`, `--dry-run`, `--debug`, `--no-color`, `--workers`, `--rate-limit`

**Sync flags:** `-f/--file`, `-e/--environment`, `-a/--all`, `--delete-missing`, `--no-auto-classify`, `--force`

**Output formatting:**
- Symbols: `✓` Created, `↻` Updated, `=` Unchanged, `⊘` Skipped, `✗` Failed
- Tags: `[masked]`, `[protected]`
- Summary: `Created: N | Updated: N | Unchanged: N | Skipped: N | Failed: N`
- Footer: `Duration: Xs | API calls: N | Rate: X.X req/s`
- Diff: `+` create, `~` update, `-` delete, `=` unchanged
- Masked values shown as `***`

**Steps:**
- [ ] Implement `GlobalOptions` struct with go-flags tags
- [ ] Implement `main()` with `flags.NewParser`, subcommand registration, `signal.NotifyContext`
- [ ] Implement `VersionCommand.Execute()` — print version from ldflags
- [ ] Implement `buildClient(global, cfg)` helper and `setupColor(noColor)`
- [ ] Implement `SyncCommand.Execute()` — full flow: config → client → parse → classify → fetch → diff → confirm → apply → report
- [ ] Implement `DiffCommand.Execute()` — same flow, print diff without apply
- [ ] Implement `ListCommand.Execute()` — fetch and print as aligned table
- [ ] Implement `ExportCommand.Execute()` — fetch and write KEY=VALUE format
- [ ] Implement `DeleteCommand.Execute()` — delete with confirmation prompt
- [ ] Implement `printSyncReport(report)` and `printDiff(diff)` with colored output
- [ ] Implement `maskIfNeeded(value, classification)` helper
- [ ] Verify: `go build ./cmd/glenv/` compiles
- [ ] Verify: `./bin/glenv version`, `./bin/glenv sync --help` work correctly

---

### Task 9: Verification

---
model: sonnet
priority: P0
complexity: Medium
---

**Steps:**
- [ ] Run `go test -race ./...` — all tests pass
- [ ] Run `go vet ./...` — no issues
- [ ] Run `make build` — binary compiles
- [ ] Manual test: `./bin/glenv version` prints version
- [ ] Manual test: `./bin/glenv sync --help` shows correct flags
- [ ] Manual test: `./bin/glenv diff --help` shows correct flags
- [ ] Manual test: `./bin/glenv list --help` shows correct flags
- [ ] Check test coverage: `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out` — target >= 80%
- [ ] Fix any issues found during verification

---

### Task 10: Release Infrastructure

---
model: haiku
priority: P1
complexity: Simple
---

**Description:** Set up goreleaser for cross-compilation and GitHub Actions for CI/CD.

**Files:**
- Create: `.goreleaser.yml`
- Create: `.github/workflows/ci.yml`
- Create: `.github/workflows/release.yml`

**Steps:**
- [ ] Create `.goreleaser.yml`:
  - Cross-compile: linux/darwin/windows × amd64/arm64
  - CGO_ENABLED=0, ldflags with version
  - Archives: tar.gz (unix), zip (windows)
  - Checksums
- [ ] Create `.github/workflows/ci.yml`:
  - Trigger: push/PR to main
  - Jobs: go test -race, go vet, golangci-lint
  - Go version matrix: [1.25]
- [ ] Create `.github/workflows/release.yml`:
  - Trigger: push tag `v*`
  - Job: goreleaser release --clean
  - GITHUB_TOKEN secret
- [ ] Verify: `make release-check` (goreleaser dry-run) passes

---

## File Summary

| Task | New Files | Modified Files |
|------|-----------|----------------|
| 1 | go.mod, go.sum, Makefile, .gitignore, LICENSE, 5x doc.go stubs | — |
| 2 | pkg/envfile/parser.go, pkg/envfile/parser_test.go | — |
| 3 | pkg/classifier/classifier.go, pkg/classifier/classifier_test.go | — |
| 4 | pkg/config/config.go, pkg/config/config_test.go | — |
| 5 | pkg/gitlab/client.go, pkg/gitlab/client_test.go | — |
| 6 | pkg/gitlab/variables.go, pkg/gitlab/variables_test.go | — |
| 7 | pkg/sync/engine.go, pkg/sync/engine_test.go | — |
| 8 | cmd/glenv/main.go | — |
| 9 | — | — (verification only) |
| 10 | .goreleaser.yml, .github/workflows/ci.yml, .github/workflows/release.yml | — |

## Risks and Assumptions

| Risk | Impact | Mitigation |
|------|--------|------------|
| Go 1.25 not available | Low | Use 1.23/1.24, no 1.25-specific features used |
| GitLab pagination edge cases | Low | Test X-Next-Page empty string and "0" as termination |
| Multiline values + masked conflict | Medium | Classifier rejects multiline for masked (GitLab API rejects them) |
| `filter[environment_scope]` URL encoding | Low | Use `url.Values` for correct `[]` encoding |
| go-flags doesn't support context in Execute | Low | Use package-level `var appCtx context.Context` |

**Assumptions:**
- GitLab API v4 is stable and endpoints match docs/SPEC.md
- `go-flags` supports all needed flag types and subcommand patterns
- Test coverage target of 80% is achievable with table-driven tests
- No group-level variables needed (project-level only per spec)

**Open Questions:**
- None — all requirements are fully specified in SPEC.md and design doc
