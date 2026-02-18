# Fix: Resolve .env file path from config when using -e flag

## Overview
When running `glenv sync -e staging` or `glenv diff -e staging`, the tool ignores the `file`
path specified in config's `environments` section and always uses the default `.env`.
This fix adds a helper function to resolve the correct file path based on priority:
explicit --file flag > config environment file > default ".env".

## Context
- **Files involved:** `cmd/glenv/main.go`
- **Existing patterns:** Helper functions like `resolveWorkers()` already exist for similar resolution logic
- **Dependencies:** None

## Development Approach
- Testing approach: regular (code then manual tests)
- Complete each task fully before moving to the next
- Verify with existing tests + manual testing

## Implementation Tasks

### Task 1: Add resolveEnvFile helper function

---
model: sonnet
priority: P0
complexity: Low
---

**Files:**
- Modify: `cmd/glenv/main.go`

**Steps:**
- [x] Add `resolveEnvFile(flagFile, environment string, cfg *config.Config) string` function after `resolveWorkers()`
- [x] Implement priority logic: explicit flag (non-empty, non-default) > config file > fallback
- [x] Run `go vet ./...` — must pass

### Task 2: Remove default:".env" from struct tags

---
model: haiku
priority: P0
complexity: Low
---

**Files:**
- Modify: `cmd/glenv/main.go` (lines 62, 171)

**Steps:**
- [ ] Remove `default:".env"` from SyncCommand.File field (line 62)
- [ ] Remove `default:".env"` from DiffCommand.File field (line 171)
- [ ] Update description to clarify default behavior
- [ ] Run `go build ./...` — must pass

### Task 3: Integrate resolveEnvFile into SyncCommand.Execute

---
model: sonnet
priority: P0
complexity: Low
---

**Files:**
- Modify: `cmd/glenv/main.go` (SyncCommand.Execute method)

**Steps:**
- [ ] Before line 108, call `resolveEnvFile()` to get correct file path
- [ ] Pass resolved path to `syncOne()` instead of `cmd.File`
- [ ] Run existing tests: `go test -race ./...` — must pass

### Task 4: Integrate resolveEnvFile into DiffCommand.Execute

---
model: sonnet
priority: P0
complexity: Low
---

**Files:**
- Modify: `cmd/glenv/main.go` (DiffCommand.Execute method)

**Steps:**
- [ ] After loading config, call `resolveEnvFile()` to get correct file path
- [ ] Use resolved path in `envfile.ParseFile()` call
- [ ] Run existing tests: `go test -race ./...` — must pass

### Task 5: Verification

---
model: haiku
priority: P0
complexity: Low
---

**Steps:**
- [ ] Run `go test -race ./...` — all tests pass
- [ ] Run `go vet ./...` — no issues
- [ ] Manual test: `glenv diff -e staging` uses config file path
- [ ] Manual test: `glenv diff -e staging -f .env` uses explicit flag
- [ ] Manual test: `glenv diff` (no -e) uses default `.env`

## File Summary

| Task | New Files | Modified Files |
|------|-----------|----------------|
| 1-4  | —         | `cmd/glenv/main.go` |

## Risks and Assumptions

| Risk | Impact | Mitigation |
|------|--------|------------|
| Breaking change for users relying on current behavior | Low | Current behavior is a bug, fix aligns with expected behavior |

**Assumptions:**
- Users expect `-e <env>` to use the file path from config if defined
- Explicit `--file` flag should always take priority over config

**Open Questions:**
- None
