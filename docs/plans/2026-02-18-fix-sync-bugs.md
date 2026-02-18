# Fix: Sync bugs - env file resolution and scope mismatch

## Overview
Two critical bugs in glenv sync command: (1) ignores .env file paths from config when using `-e` flag,
(2) incorrectly determines UPDATE instead of CREATE when variable exists with different environment scope,
causing 404 errors.

## Context
- **Files involved:** `cmd/glenv/main.go`, `pkg/sync/engine.go`, `pkg/sync/engine_test.go`
- **Existing patterns:** Helper functions like `resolveWorkers()` for config resolution
- **Dependencies:** None

## Development Approach
- Testing approach: regular (code then tests)
- Complete each task fully before moving to the next
- Every task includes writing/updating tests
- All tests must pass before starting the next task

## Implementation Tasks

### Task 1: Add resolveEnvFile helper function

---
model: sonnet
priority: P0
complexity: Low
---

**Description:** Add helper to resolve .env file path with priority: explicit flag > config > default

**Files:**
- Modify: `cmd/glenv/main.go`

**Steps:**
- [x] Add `resolveEnvFile(flagFile, environment string, cfg *config.Config) string` after `resolveWorkers()`
- [x] Implement priority logic: non-empty flag → config env file → ".env" default
- [x] Run `go vet ./...` — must pass

### Task 2: Remove default:".env" from struct tags

---
model: haiku
priority: P0
complexity: Low
---

**Files:**
- Modify: `cmd/glenv/main.go`

**Steps:**
- [ ] Remove `default:".env"` from SyncCommand.File (line 62)
- [ ] Remove `default:".env"` from DiffCommand.File (line 171)
- [ ] Update description to clarify default behavior
- [ ] Run `go build ./...` — must pass

### Task 3: Integrate resolveEnvFile into SyncCommand.Execute

---
model: sonnet
priority: P0
complexity: Low
---

**Files:**
- Modify: `cmd/glenv/main.go`

**Steps:**
- [ ] Before line 108, call `resolveEnvFile()` to get correct file path
- [ ] Pass resolved path to `syncOne()` instead of `cmd.File`
- [ ] Run `go test -race ./...` — must pass

### Task 4: Integrate resolveEnvFile into DiffCommand.Execute

---
model: sonnet
priority: P0
complexity: Low
---

**Files:**
- Modify: `cmd/glenv/main.go`

**Steps:**
- [ ] After loading config, call `resolveEnvFile()` to get correct file path
- [ ] Use resolved path in `envfile.ParseFile()` call
- [ ] Run `go test -race ./...` — must pass

### Task 5: Fix Diff() scope matching logic

---
model: sonnet
priority: P0
complexity: Medium
---

**Description:** Fix bug where UPDATE is determined for variable with mismatched scope instead of CREATE

**Files:**
- Modify: `pkg/sync/engine.go`

**Steps:**
- [ ] In Diff() function, after `rv, exists := remoteMap[lv.Key]`
- [ ] Add scope match check: `scopeMatch := exists && (rv.EnvironmentScope == envScope || rv.EnvironmentScope == "*")`
- [ ] Change switch to use `!scopeMatch` for CREATE case instead of `!exists`
- [ ] For UPDATE case, use `rv.EnvironmentScope` instead of `envScope` parameter
- [ ] Run `go test -race ./...` — must pass

### Task 6: Add unit test for scope mismatch scenario

---
model: sonnet
priority: P1
complexity: Low
---

**Files:**
- Modify: `pkg/sync/engine_test.go`

**Steps:**
- [ ] Add test: local var exists, remote var exists with different scope → expect CREATE
- [ ] Add test: local var exists, remote var exists with wildcard "*" scope → expect UPDATE
- [ ] Run `go test -race ./pkg/sync/...` — must pass

### Task 7: Verification

---
model: haiku
priority: P0
complexity: Low
---

**Steps:**
- [ ] Run `go test -race ./...` — all tests pass
- [ ] Run `go vet ./...` — no issues
- [ ] Manual test: `glenv diff -e staging` uses config file path
- [ ] Manual test: `glenv diff -e production` with staging-only var shows CREATE not UPDATE
- [ ] Manual test: `glenv sync -e production` creates new variable (no 404)

## File Summary

| Task | New Files | Modified Files |
|------|-----------|----------------|
| 1-4  | —         | `cmd/glenv/main.go` |
| 5    | —         | `pkg/sync/engine.go` |
| 6    | —         | `pkg/sync/engine_test.go` |

## Risks and Assumptions

| Risk | Impact | Mitigation |
|------|--------|------------|
| GitLab API filter behavior varies | Medium | Handle scope mismatch in code regardless of API behavior |

**Assumptions:**
- Users expect `-e <env>` to use file path from config if defined
- When variable exists with different scope, CREATE new variable for target scope
- Wildcard scope "*" matches all environments

**Open Questions:**
- None
