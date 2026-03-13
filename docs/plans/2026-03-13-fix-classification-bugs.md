# Fix variable_type=file and masked=false Classification Bugs

## Overview
Fix two related bugs: (1) base64-encoded SSH keys incorrectly classified as `file` instead of `env_var` due to key name pattern matching, and (2) manually-set `masked=true` flag being reset to `false` on sync. Root cause is that `matchesFile()` checks key name only, ignoring value structure.

## Context
- **Files involved:** `pkg/classifier/classifier.go`, `pkg/sync/engine.go`, tests
- **Existing patterns:** `protected` flag has floor logic (`cl.Protected || rv.Protected`) — same pattern needed for `masked`
- **Dependencies:** None external

## Development Approach
- Testing approach: regular (code then tests)
- Complete each task fully before moving to the next
- Every task includes writing/updating tests
- All tests must pass before starting the next task

## Implementation Tasks

### Task 1: Fix matchesFile() — require newlines for key patterns

---
model: sonnet
priority: P0
complexity: Medium
---

**Description:** Key pattern matching (PRIVATE_KEY, _CERT, _PEM) should only return `file` type if value contains real newlines. Base64 values are single-line → should stay `env_var`.

**Files:**
- Modify: `pkg/classifier/classifier.go` (lines 139-160)
- Modify: `pkg/classifier/classifier_test.go`

**Steps:**
- [x] In `matchesFile()`, add check: `if !strings.Contains(value, "\n") { return false }` before key pattern loop
- [x] PEM header detection (`-----BEGIN`) remains unchanged (works without newlines)
- [x] Add test: `TestClassify_Base64PrivateKey_EnvVar` — base64 value with PRIVATE_KEY name → env_var
- [x] Add test: `TestClassify_MultilinePrivateKey_FileType` — PEM with newlines → file
- [x] Run tests — must pass

### Task 2: Add PRIVATE_KEY to maskedPatterns

---
model: haiku
priority: P0
complexity: Low
---

**Description:** Base64 SSH keys should be masked. Add PRIVATE_KEY to built-in masked patterns.

**Files:**
- Modify: `pkg/classifier/classifier.go` (line 34)
- Modify: `pkg/classifier/classifier_test.go`

**Steps:**
- [x] Add `"PRIVATE_KEY"` to `builtinMaskedPatterns`
- [x] Add test: `TestClassify_Base64PrivateKey_Masked` — base64 value → masked=true
- [x] Run tests — must pass

### Task 3: Export IsMaskable function

---
model: haiku
priority: P1
complexity: Low
---

**Description:** Export `isMaskable` as `IsMaskable` for use in sync engine.

**Files:**
- Modify: `pkg/classifier/classifier.go` (after line 117)

**Steps:**
- [x] Add exported wrapper: `func IsMaskable(value string) bool { return isMaskable(value) }`
- [x] Run tests — must pass

### Task 4: Add floor logic for masked in Diff UPDATE

---
model: sonnet
priority: P0
complexity: Medium
---

**Description:** Preserve manually-set `masked=true` on update, mirroring `protected` floor logic.

**Files:**
- Modify: `pkg/sync/engine.go` (line 159)
- Modify: `pkg/sync/engine_test.go`

**Steps:**
- [x] Change `masked: cl.Masked` to `masked: cl.Masked || (rv.Masked && classifier.IsMaskable(lv.Value))`
- [x] Add import for classifier package if needed
- [x] Add test: `TestDiff_PreserveMaskedOnUpdate` — remote masked=true, classifier masked=false → preserve true
- [x] Run tests — must pass

### Task 5: Verification

---
model: sonnet
priority: P0
complexity: Low
---

- [x] Run full test suite: `go test -race ./...`
- [x] Run linter: `make lint`
- [x] Manual test with base64 SSH key in `.env.production`
- [x] Verify `glenv diff -e production` shows `env_var,masked,protected`

## File Summary

| Task | New Files | Modified Files |
|------|-----------|----------------|
| 1    | —         | `pkg/classifier/classifier.go`, `pkg/classifier/classifier_test.go` |
| 2    | —         | `pkg/classifier/classifier.go`, `pkg/classifier/classifier_test.go` |
| 3    | —         | `pkg/classifier/classifier.go` |
| 4    | —         | `pkg/sync/engine.go`, `pkg/sync/engine_test.go` |

## Risks and Assumptions

| Risk | Impact | Mitigation |
|------|--------|------------|
| Breaking existing PEM detection | Medium | PEM header (`-----BEGIN`) detection unchanged, only key-pattern path affected |
| Tests with PRIVATE_KEY name may fail | Low | Update affected tests to use multiline values |

**Assumptions:**
- Base64 values never contain literal newlines
- GitLab API correctly validates maskable values

**Open Questions:**
- None
