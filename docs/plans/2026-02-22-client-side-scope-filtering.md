# Client-side Environment Scope Filtering

## Overview
GitLab API ignores `filter[environment_scope]` parameter for LIST variables endpoint. This causes glenv to incorrectly identify existing variables as CREATE instead of UPDATE, resulting in "key already taken" errors. The fix adds client-side filtering after fetching all variables.

## Context
- **Files involved:** `pkg/gitlab/variables.go`, `pkg/sync/engine.go`, `cmd/glenv/main.go`, `pkg/gitlab/variables_test.go`, `pkg/sync/engine_test.go`
- **Existing patterns:** Utility functions in pkg/gitlab, filtering logic in Diff()
- **Dependencies:** None (internal fix)
- **Reference:** https://gitlab.com/gitlab-org/gitlab/-/issues/343169

## Development Approach
- Testing approach: Regular (code then tests)
- Complete each task fully before moving to the next
- Every task includes writing/updating tests
- All tests must pass before starting the next task

## Implementation Tasks

### Task 1: Add FilterByScope Function

---
model: sonnet
priority: P0
complexity: Low
---

**Files:**
- Modify: `pkg/gitlab/variables.go`
- Modify: `pkg/gitlab/variables_test.go`

**Steps:**
- [x] Add `FilterByScope(vars []Variable, scope string) []Variable` function after Variable struct
- [x] Implement logic: empty scope → return all; scope="*" → only wildcard; specific scope → exact + wildcard
- [x] Add tests: ExactMatch, WildcardTarget, EmptyScope, NoMatches, EmptyInput
- [x] Run `go test ./pkg/gitlab/...` — must pass

### Task 2: Apply Filter in Diff Engine

---
model: sonnet
priority: P0
complexity: Low
---

**Files:**
- Modify: `pkg/sync/engine.go`
- Modify: `pkg/sync/engine_test.go`

**Steps:**
- [x] Import `gitlab` package if not already imported
- [x] Call `gitlab.FilterByScope(remote, envScope)` at start of `Diff()`
- [x] Update comment explaining client-side filtering
- [x] Add regression tests: FiltersRemoteByScope, IgnoresOtherScopes
- [x] Run `go test ./pkg/sync/...` — must pass

### Task 3: Apply Filter in List and Export Commands

---
model: haiku
priority: P1
complexity: Low
---

**Files:**
- Modify: `cmd/glenv/main.go`

**Steps:**
- [ ] Add `vars = gitlab.FilterByScope(vars, cmd.Environment)` after ListVariables in ListCommand
- [ ] Add `vars = gitlab.FilterByScope(vars, cmd.Environment)` after ListVariables in ExportCommand
- [ ] Run `go test ./cmd/glenv/...` — must pass

### Task 4: Verification

---
model: haiku
priority: P0
complexity: Low
---

**Steps:**
- [ ] Run full test suite: `go test -race ./...`
- [ ] Run `go vet ./...`
- [ ] Manual test: `glenv list -e production` — should show only production + wildcard vars
- [ ] Manual test: `glenv sync -e production --dry-run` — should show UPDATE not CREATE for existing vars

## File Summary

| Task | New Files | Modified Files |
|------|-----------|----------------|
| 1    | —         | `pkg/gitlab/variables.go`, `pkg/gitlab/variables_test.go` |
| 2    | —         | `pkg/sync/engine.go`, `pkg/sync/engine_test.go` |
| 3    | —         | `cmd/glenv/main.go` |
| 4    | —         | — |

## Risks and Assumptions

| Risk | Impact | Mitigation |
|------|--------|------------|
| Filter logic edge cases | Wrong vars filtered | Comprehensive unit tests |

**Assumptions:**
- Wildcard `*` variables should match any specific scope (standard GitLab behavior)
- Empty scope means "no filtering" (return all variables)

**Open Questions:**
- None
