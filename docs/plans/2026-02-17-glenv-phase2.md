# glenv Phase 2 — Networking, Engine, CLI & Release

## Overview

Phase 2 of glenv implementation: GitLab API client with rate limiting and retry, sync engine with concurrent worker pool, CLI composition with all 6 commands and colored output, and release infrastructure. This phase connects all foundation packages from Phase 1 into a working CLI tool.

## Context

- **Prerequisite:** Phase 1 complete — `pkg/envfile`, `pkg/classifier`, `pkg/config` implemented with passing tests
- **Files involved:** `pkg/gitlab/`, `pkg/sync/`, `cmd/glenv/`, `.goreleaser.yml`, `.github/workflows/`
- **Existing patterns:** TDD approach, table-driven tests, types from Phase 1 packages
- **Dependencies:** `golang.org/x/time/rate`, `github.com/jessevdk/go-flags`, `github.com/fatih/color`

## Development Approach

- Testing approach: **TDD** — write tests first, then implementation
- Complete each task fully before moving to the next
- Every task includes writing tests before code
- `go test -race ./...` must pass before starting the next task

---

## Implementation Tasks

### Task 1: GitLab HTTP Client — Core + Rate Limiter

---
model: opus
priority: P0
complexity: Large
---

**Description:** Implement the core HTTP client with token bucket rate limiting. This is the foundation for all GitLab API interactions.

**Files:**
- Create: `pkg/gitlab/client.go`
- Create: `pkg/gitlab/client_test.go`

**Types to implement:**
- `ClientConfig{BaseURL, Token string, RequestsPerSecond float64, Burst int, RetryMax int, RetryInitialBackoff time.Duration, HTTPClient *http.Client}`
- `Client{cfg ClientConfig, limiter *rate.Limiter, http *http.Client}`

**Steps:**
- [x] Write mock HTTP server test helper: `setupMockServer(t, handler) (*httptest.Server, *Client)` with high rate limit for tests
- [x] Write tests:
  - `TestNewClient_Defaults` — verify limiter created with correct rate/burst
  - `TestDo_AuthHeader` — mock server verifies `PRIVATE-TOKEN` header value
  - `TestDo_RateLimiting` — verify `limiter.Wait(ctx)` is called per request
  - `TestDo_Success` — 200 response body decoded correctly
  - `TestDo_ContextCancel` — cancelled context returns error immediately
- [x] Implement `NewClient(cfg ClientConfig) *Client`:
  - Default burst = int(RequestsPerSecond) + 1 if not set
  - Default HTTPClient with 30s timeout if nil
  - Create `rate.NewLimiter(rate.Limit(rps), burst)`
- [x] Implement `do(ctx context.Context, req *http.Request) (*http.Response, error)`:
  - Call `c.limiter.Wait(ctx)` before each request
  - Set `PRIVATE-TOKEN` and `Content-Type: application/json` headers
  - Execute request via `c.http.Do(req)`
  - Return response (retry logic added in Task 2)
- [x] Run `go test -race ./pkg/gitlab/` — all tests pass

---

### Task 2: GitLab HTTP Client — Retry & Error Handling

---
model: opus
priority: P0
complexity: Medium
---

**Description:** Extend the HTTP client with exponential backoff, jitter, 429/Retry-After handling, 401 hard stop, and request body cloning for retries.

**Files:**
- Modify: `pkg/gitlab/client.go`
- Modify: `pkg/gitlab/client_test.go`

**Steps:**
- [x] Write tests:
  - `TestDo_Retry_NetworkError` — mock server drops connection → verify 3 retry attempts, then error
  - `TestDo_Retry_429_RetryAfter` — mock returns 429 with `Retry-After: 1` → verify client waits then retries
  - `TestDo_Retry_429_NoHeader` — 429 without header → use default backoff
  - `TestDo_401_NoRetry` — mock returns 401 → immediate error, verify only 1 request sent
  - `TestDo_MaxRetriesExceeded` — always fails → verify error after RetryMax+1 attempts
  - `TestBackoff_ExponentialWithJitter` — verify backoff grows exponentially, includes jitter component
- [x] Implement `backoff(attempt int, extra time.Duration) time.Duration`:
  - Formula: `RetryInitialBackoff * 2^attempt + random(0, 500ms)`
  - Use `math/rand` for jitter
- [x] Implement `parseRetryAfter(resp *http.Response) time.Duration`:
  - Parse `Retry-After` header as integer seconds
  - Fallback to `RetryInitialBackoff` if header missing or unparseable
- [x] Implement `cloneRequest(req *http.Request) (*http.Request, error)`:
  - Clone request with `req.Clone(req.Context())`
  - Buffer and restore body for replay (read body, create two `io.NopCloser(bytes.NewReader)`)
- [x] Implement `sleep(ctx context.Context, d time.Duration) error`:
  - `select { case <-time.After(d): return nil; case <-ctx.Done(): return ctx.Err() }`
- [x] Extend `do()` with retry loop:
  - Loop `attempt := 0; attempt <= c.cfg.RetryMax; attempt++`
  - Clone request before each attempt
  - On 401: close body, return auth error immediately (no retry)
  - On 429: close body, parse Retry-After, sleep, continue loop
  - On network error: sleep with backoff, continue loop
  - On success: return response
  - After loop exhausted: return last error wrapped with "max retries exceeded"
- [x] Run `go test -race ./pkg/gitlab/` — all tests pass

---

### Task 3: GitLab Variables — CRUD Operations

---
model: sonnet
priority: P0
complexity: Medium
---

**Description:** Implement CRUD operations for GitLab CI/CD variables with pagination and environment scope filtering.

**Files:**
- Create: `pkg/gitlab/variables.go`
- Create: `pkg/gitlab/variables_test.go`

**Types to implement:**
- `Variable{Key, Value, VariableType, EnvironmentScope string, Protected, Masked, Raw bool}` — JSON tags matching GitLab API
- `CreateRequest{Key, Value, VariableType, EnvironmentScope string, Protected, Masked bool}` — JSON tags for POST/PUT body
- `ListOptions{EnvironmentScope string}`

**API endpoints:**
- GET `/api/v4/projects/:id/variables` — paginated, `per_page=100`
- GET `/api/v4/projects/:id/variables/:key`
- POST `/api/v4/projects/:id/variables`
- PUT `/api/v4/projects/:id/variables/:key`
- DELETE `/api/v4/projects/:id/variables/:key`
- All support `filter[environment_scope]=<scope>` query parameter

**Steps:**
- [x] Write tests with mock server:
  - `TestListVariables_SinglePage` — mock returns 3 vars, no `X-Next-Page` header → returns all 3
  - `TestListVariables_MultiPage` — page 1 returns 2 vars + `X-Next-Page: 2`, page 2 returns 1 var + no header → returns all 3
  - `TestListVariables_WithEnvScope` — verify `filter[environment_scope]=production` query param sent
  - `TestListVariables_Empty` — mock returns `[]` → returns empty slice
  - `TestCreateVariable` — verify POST body matches `CreateRequest` JSON, response parsed to `Variable`
  - `TestUpdateVariable` — verify PUT to `/variables/:key`, body correct
  - `TestDeleteVariable` — verify DELETE to `/variables/:key`, returns no error on 204
  - `TestGetVariable_Found` — 200 with JSON → returns `*Variable`
  - `TestGetVariable_NotFound` — 404 → returns `nil, nil` (not an error)
- [x] Implement `ListVariables(ctx context.Context, projectID string, opts ListOptions) ([]Variable, error)`:
  - Pagination loop: start page=1, per_page=100
  - Build URL with `url.Values` for correct `filter[environment_scope]` encoding
  - Check `X-Next-Page` header — break if empty string or not present
  - Accumulate all variables across pages
- [x] Implement `GetVariable(ctx, projectID, key, envScope string) (*Variable, error)`:
  - 404 → return nil, nil
  - Success → decode and return
- [x] Implement `CreateVariable(ctx, projectID string, req CreateRequest) (*Variable, error)`:
  - POST with JSON body, decode response
- [x] Implement `UpdateVariable(ctx, projectID string, req CreateRequest) (*Variable, error)`:
  - PUT to `/variables/:key` with JSON body
- [x] Implement `DeleteVariable(ctx, projectID, key, envScope string) error`:
  - DELETE, add `filter[environment_scope]` if envScope non-empty
- [x] Run `go test -race ./pkg/gitlab/` — all tests pass

---

### Task 4: Sync Engine — Diff Calculation

---
model: sonnet
priority: P0
complexity: Medium
---

**Description:** Implement diff types and diff calculation algorithm that compares local .env variables against remote GitLab variables.

**Files:**
- Create: `pkg/sync/engine.go`
- Create: `pkg/sync/engine_test.go`

**Types to implement:**
- `ChangeKind string` with constants: `ChangeCreate`, `ChangeUpdate`, `ChangeDelete`, `ChangeUnchanged`, `ChangeSkipped`
- `Change{Kind ChangeKind, Key, OldValue, NewValue string, Classification classifier.Classification, SkipReason string}`
- `DiffResult{Changes []Change}`
- `Result{Change Change, Error error}`
- `SyncReport{Created, Updated, Deleted, Unchanged, Skipped, Failed, APICalls int, Duration time.Duration, Errors []error}`
- `Options{Workers int, DryRun, DeleteMissing bool, Environment string}`
- `Engine{client *gitlab.Client, classifier *classifier.Classifier, opts Options, projectID string}`

**Steps:**
- [x] Write tests:
  - `TestDiff_CreateNew` — local has `DB_HOST`, remote empty → Change{Kind: ChangeCreate, Key: "DB_HOST"}
  - `TestDiff_UpdateChanged` — local `API_KEY=new`, remote `API_KEY=old` → Change{Kind: ChangeUpdate, OldValue: "old", NewValue: "new"}
  - `TestDiff_Unchanged` — local and remote identical → Change{Kind: ChangeUnchanged}
  - `TestDiff_DeleteMissing_Enabled` — remote has `OLD_VAR` not in local, DeleteMissing=true → Change{Kind: ChangeDelete}
  - `TestDiff_DeleteMissing_Disabled` — same scenario, DeleteMissing=false → no ChangeDelete in result
  - `TestDiff_MultipleChanges` — mix of create, update, unchanged → all correctly classified
  - `TestDiff_Classification` — verify classifier is called and Classification populated in changes
- [x] Implement `NewEngine(client *gitlab.Client, cls *classifier.Classifier, opts Options, projectID string) *Engine`
- [x] Implement `Diff(ctx context.Context, local []envfile.Variable, remote []gitlab.Variable, envScope string) DiffResult`:
  - Build map of remote vars by key
  - Iterate local: if in remote and same value → Unchanged; if different → Update; if not in remote → Create
  - Classify each Create/Update change via `e.classifier.Classify()`
  - If DeleteMissing: iterate remote, if not in local map → Delete
- [x] Run `go test -race ./pkg/sync/` — all tests pass

---

### Task 5: Sync Engine — Worker Pool & Apply

---
model: opus
priority: P0
complexity: Large
---

**Description:** Implement concurrent worker pool for applying diff changes to GitLab. Includes graceful shutdown, dry-run mode, streaming callback, and thread-safe result collection.

**Files:**
- Modify: `pkg/sync/engine.go`
- Modify: `pkg/sync/engine_test.go`

**Steps:**
- [ ] Write tests:
  - `TestApply_DryRun` — DryRun=true, verify no HTTP requests made, report shows correct counts
  - `TestApply_Concurrent` — 20 changes, 5 workers, run with `-race` → all changes processed, no races
  - `TestApply_CreateUpdateDelete` — mix of 3 types → verify correct API calls made (mock server counts)
  - `TestApply_ContextCancel` — cancel context after 2 results → verify partial report (some completed, rest not)
  - `TestApply_ErrorHandling` — mock server returns 500 for one key → Failed=1, others succeed
  - `TestApplyWithCallback` — verify callback called for each result in real-time
  - `TestApply_EmptyDiff` — no actionable changes → immediate return with zero counts
- [ ] Implement `applyOne(ctx context.Context, task Change) Result`:
  - Switch on task.Kind:
    - ChangeCreate → `e.client.CreateVariable(ctx, e.projectID, createRequest)`
    - ChangeUpdate → `e.client.UpdateVariable(ctx, e.projectID, updateRequest)`
    - ChangeDelete → `e.client.DeleteVariable(ctx, e.projectID, task.Key, e.opts.Environment)`
  - Map Classification fields to CreateRequest fields
  - Return `Result{Change: task, Error: err}`
- [ ] Implement `Apply(ctx context.Context, diff DiffResult) SyncReport`:
  - Filter out ChangeUnchanged from actionable tasks
  - If DryRun: return report with counts but no API calls
  - Create `taskCh chan Change` (buffered), `resultCh chan Result` (buffered)
  - Launch `opts.Workers` goroutines reading from taskCh, calling applyOne, sending to resultCh
  - Feed all actionable tasks to taskCh, close it
  - Wait (via WaitGroup) then close resultCh
  - Collect results: increment Created/Updated/Deleted/Failed counters
  - Add Unchanged count from diff
  - Set Duration, APICalls
- [ ] Implement `ApplyWithCallback(ctx context.Context, diff DiffResult, cb func(Result)) SyncReport`:
  - Same as Apply but calls `cb(result)` for each result as it arrives (before collecting)
  - Enables streaming output in CLI
- [ ] Verify graceful shutdown: context cancellation causes `limiter.Wait(ctx)` to return error → workers exit → partial results collected
- [ ] Run `go test -race ./pkg/sync/` — all tests pass

---

### Task 6: CLI — Structure, Global Flags, Version

---
model: sonnet
priority: P0
complexity: Medium
---

**Description:** Set up the CLI entry point with go-flags, global options, signal handling, and the version command. This creates the executable skeleton.

**Files:**
- Create: `cmd/glenv/main.go`

**Steps:**
- [ ] Implement `GlobalOptions` struct with go-flags tags:
  - `Config string` — `short:"c" long:"config" description:"Config file path"`
  - `Token string` — `long:"token" env:"GITLAB_TOKEN"`
  - `Project string` — `long:"project" env:"GITLAB_PROJECT_ID"`
  - `URL string` — `long:"url" env:"GITLAB_URL" default:"https://gitlab.com"`
  - `DryRun bool` — `short:"n" long:"dry-run"`
  - `Debug bool` — `short:"d" long:"debug"`
  - `NoColor bool` — `long:"no-color" env:"NO_COLOR"`
  - `Workers int` — `short:"w" long:"workers" default:"5"`
  - `RateLimit float64` — `long:"rate-limit" default:"10"`
- [ ] Implement `var version = "dev"` (injected via ldflags at build time)
- [ ] Implement `var appCtx context.Context` — package-level context for subcommands
- [ ] Implement `main()`:
  - Create `signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)` → assign to `appCtx`
  - Create `flags.NewParser(&globalOpts, flags.Default)`
  - Register subcommands: sync, diff, list, export, delete, version
  - Call `parser.Parse()`, handle `flags.ErrHelp` gracefully
- [ ] Implement `VersionCommand`:
  - `Execute(args []string) error` → `fmt.Printf("glenv version %s\n", version)`
- [ ] Implement `setupColor(noColor bool)`:
  - Set `color.NoColor = true` if `noColor || os.Getenv("NO_COLOR") != ""`
- [ ] Implement `buildClient(global *GlobalOptions, cfg *config.Config) *gitlab.Client`:
  - Override cfg fields with non-zero global flags
  - Return `gitlab.NewClient(...)` with merged config
- [ ] Implement `coalesce(a, b string) string` — return first non-empty
- [ ] Verify: `go build ./cmd/glenv/` compiles
- [ ] Verify: `.bin/glenv version` prints "glenv version dev"
- [ ] Verify: `.bin/glenv --help` shows global flags and subcommand list

---

### Task 7: CLI — sync & diff Commands

---
model: opus
priority: P0
complexity: Large
---

**Description:** Implement the two primary commands: `sync` (apply changes) and `diff` (preview changes). These are the core user-facing features.

**Files:**
- Modify: `cmd/glenv/main.go`

**SyncCommand flags:**
- `File string` — `short:"f" long:"file"`
- `Environment string` — `short:"e" long:"environment"`
- `All bool` — `short:"a" long:"all"`
- `DeleteMissing bool` — `long:"delete-missing"`
- `NoAutoClassify bool` — `long:"no-auto-classify"`
- `Force bool` — `long:"force"`

**DiffCommand flags:**
- `File string` — `short:"f" long:"file"`
- `Environment string` — `short:"e" long:"environment"`

**Steps:**
- [ ] Implement `SyncCommand.Execute(args []string) error`:
  - Load config: `config.Load(global.Config)`
  - Apply CLI flag overrides to config
  - Validate config: `cfg.Validate()`
  - Setup color: `setupColor(global.NoColor)`
  - Build client: `buildClient(global, cfg)`
  - If `--all`: iterate `cfg.Environments`, sync each one
  - Otherwise: parse .env file via `envfile.ParseFile(s.File)`
  - Create classifier (with cfg.Classify rules, or empty if `--no-auto-classify`)
  - Fetch remote: `client.ListVariables(appCtx, projectID, opts)`
  - Calculate diff: `engine.Diff(appCtx, local, remote, envScope)`
  - Print diff summary
  - If `--delete-missing` and `!--force`: prompt "Delete N variables? [y/N]" via `bufio.Scanner(os.Stdin)`
  - Apply: `engine.ApplyWithCallback(appCtx, diff, printResult)`
  - Print summary report
- [ ] Implement `DiffCommand.Execute(args []string) error`:
  - Same flow as sync up to diff calculation
  - Call `printDiff(diff)` — display-only, no apply
  - Print summary: `Created: N | Updated: N | Deleted: N | Unchanged: N`
- [ ] Implement `printDiff(diff sync.DiffResult)`:
  - `+` green for create: `+ KEY = "value"`
  - `~` yellow for update: `~ KEY = "old" → "new"`
  - `-` red for delete: `- KEY`
  - `=` cyan for unchanged: `= KEY = "value"`
  - Masked values shown as `***`
- [ ] Handle `--all` mode: iterate environments map from config, sync each sequentially
- [ ] Verify: `./bin/glenv sync --help` shows all flags
- [ ] Verify: `./bin/glenv diff --help` shows all flags

---

### Task 8: CLI — list, export, delete Commands

---
model: sonnet
priority: P0
complexity: Medium
---

**Description:** Implement secondary commands for listing, exporting, and deleting GitLab CI/CD variables.

**Files:**
- Modify: `cmd/glenv/main.go`

**Steps:**
- [ ] Implement `ListCommand`:
  - Flags: `Environment string` — `short:"e" long:"environment"`
  - `Execute()`: fetch variables, print as aligned table:
    ```
    SCOPE        KEY                TYPE      MASKED    PROTECTED
    production   DB_HOST            env_var   false     false
    production   DB_PASSWORD        env_var   true      true
    ```
  - Use `text/tabwriter` for alignment
  - If `-e` specified: filter by environment scope
- [ ] Implement `ExportCommand`:
  - Flags: `Environment string` — `short:"e" long:"environment"`, `Output string` — `short:"o" long:"output"`
  - `Execute()`: fetch variables, format as `KEY="VALUE"` (quote if contains spaces/special chars)
  - Write to `--output` file or stdout if not specified
  - Skip file-type variables in export (or export with comment)
- [ ] Implement `DeleteCommand`:
  - Flags: `Environment string` — `short:"e" long:"environment"`, `Key string` — `long:"key"`, `Force bool` — `long:"force"`
  - `Execute()`: validate `--key` is provided
  - If `!--force`: prompt "Delete variable KEY from SCOPE? [y/N]"
  - Delete via `client.DeleteVariable()`
  - Print result: `✓ Deleted: KEY` or `✗ Failed: KEY (error)`
- [ ] Implement `confirmPrompt(message string) bool`:
  - Read line from stdin, return true if "y" or "Y"
  - Used by both sync (delete-missing) and delete commands
- [ ] Verify: `./bin/glenv list --help`, `export --help`, `delete --help` work

---

### Task 9: CLI — Colored Output & Report Formatting

---
model: sonnet
priority: P0
complexity: Small
---

**Description:** Implement colored output helpers for sync reports, using fatih/color with symbols and tags.

**Files:**
- Modify: `cmd/glenv/main.go`

**Output format (from spec):**
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

**Steps:**
- [ ] Define color variables:
  - `green = color.New(color.FgGreen)` — for created
  - `yellow = color.New(color.FgYellow)` — for updated
  - `cyan = color.New(color.FgCyan)` — for unchanged
  - `red = color.New(color.FgRed)` — for failed/deleted
  - `gray = color.New(color.FgHiBlack)` — for skipped
- [ ] Implement `printResultLine(r sync.Result)`:
  - Switch on r.Change.Kind: use appropriate symbol and color
  - Append tags: `[masked]` if Classification.Masked, `[protected]` if Classification.Protected
  - If error: append `(error message)`
- [ ] Implement `printSyncReport(report sync.SyncReport, file, projectID, env string)`:
  - Header: `Syncing: FILE → project PROJECTID (ENV)`
  - Separator line: `━━━...`
  - Summary: `Created: N | Updated: N | ...`
  - Footer: `Duration: Xs | API calls: N | Rate: X.X req/s`
- [ ] Implement `maskIfNeeded(value string, cls classifier.Classification) string`:
  - Return `"***"` if `cls.Masked`, else return value
- [ ] Implement `printHeader()`:
  - Print `glenv vVERSION` and newline
- [ ] Integrate with SyncCommand: use `ApplyWithCallback` → `printResultLine` for streaming, then `printSyncReport`
- [ ] Verify: build and manually check colored output format

---

### Task 10: Release Infrastructure

---
model: haiku
priority: P1
complexity: Simple
---

**Description:** Set up goreleaser for cross-compilation and GitHub Actions for CI/CD (test + lint on PR, release on tag).

**Files:**
- Create: `.goreleaser.yml`
- Create: `.github/workflows/ci.yml`
- Create: `.github/workflows/release.yml`

**Steps:**
- [ ] Create `.goreleaser.yml`:
  - `version: 2`
  - Build: `main: ./cmd/glenv`, `binary: glenv`, `CGO_ENABLED=0`
  - GOOS: linux, darwin, windows
  - GOARCH: amd64, arm64
  - LDFLAGS: `-s -w -X main.version={{.Version}}`
  - Archives: `name_template: "{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"`, zip for windows
  - Checksum: `checksums.txt`
  - Changelog: sort asc, exclude `docs:`, `test:`, `ci:` prefixes
- [ ] Create `.github/workflows/ci.yml`:
  - Trigger: push to main, PR to main
  - Jobs:
    - `test`: setup Go 1.25, `go mod download`, `go vet ./...`, `go test -race -coverprofile=coverage.out ./...`
    - `lint`: `golangci/golangci-lint-action@v6` with latest version
- [ ] Create `.github/workflows/release.yml`:
  - Trigger: push tag `v*`
  - Job: checkout (fetch-depth: 0), setup Go 1.25, `goreleaser/goreleaser-action@v6` with `release --clean`
  - Permissions: `contents: write`
  - Env: `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}`
- [ ] Verify: `make release-check` (goreleaser snapshot dry-run) passes

---

## File Summary

| Task | New Files | Modified Files |
|------|-----------|----------------|
| 1 | pkg/gitlab/client.go, pkg/gitlab/client_test.go | — |
| 2 | — | pkg/gitlab/client.go, pkg/gitlab/client_test.go |
| 3 | pkg/gitlab/variables.go, pkg/gitlab/variables_test.go | — |
| 4 | pkg/sync/engine.go, pkg/sync/engine_test.go | — |
| 5 | — | pkg/sync/engine.go, pkg/sync/engine_test.go |
| 6 | cmd/glenv/main.go | — |
| 7 | — | cmd/glenv/main.go |
| 8 | — | cmd/glenv/main.go |
| 9 | — | cmd/glenv/main.go |
| 10 | .goreleaser.yml, .github/workflows/ci.yml, .github/workflows/release.yml | — |

## Risks and Assumptions

| Risk | Impact | Mitigation |
|------|--------|------------|
| GitLab pagination edge cases | Low | Test `X-Next-Page` empty/"0" as loop termination |
| `filter[environment_scope]` URL encoding | Low | Use `url.Values` for correct bracket encoding |
| go-flags context limitation | Low | Package-level `appCtx` variable, set before `parser.Parse()` |
| Worker pool race conditions | Medium | Channel-based design (no shared mutable state), test with `-race` |
| Large variable sets (1000+) | Low | Pagination handles any size; rate limiter prevents 429 |
| Multiline values and masked conflict | Medium | Classifier rejects multiline for masked (GitLab API limitation) |

**Assumptions:**
- Phase 1 packages (`envfile`, `classifier`, `config`) are fully implemented and tested
- GitLab API v4 is stable — endpoints and response formats match SPEC.md
- `go-flags` subcommand `Execute()` method pattern works for all commands
- `text/tabwriter` is sufficient for list command alignment (no external dep needed)

**Open Questions:**
- None — all requirements fully specified in SPEC.md
