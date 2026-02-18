# glenv — Build & Install Infrastructure (ralphex pattern)

## Overview

Replace minimal build infrastructure with battle-tested patterns from the ralphex project: smart git-based version computation, `resolveVersion()` fallback chain, proper install/uninstall targets, and extended goreleaser config with nfpms (deb/rpm) and Homebrew tap support.

## Context

- **Files involved:** `Makefile`, `cmd/glenv/main.go`, `.goreleaser.yml`, `.gitignore`
- **Existing patterns:** ralphex Makefile (smart REV), `resolveVersion()` in `cmd/ralphex/main.go`, `.goreleaser.yml` with nfpms/brews
- **Dependencies:** None new — uses existing `runtime/debug` stdlib package

## Development Approach

- Testing approach: regular (code then verify)
- Complete each task fully before moving to the next
- Verify each target works after Makefile changes
- `make build && .bin/glenv version` must work after Task 2

---

## Implementation Tasks

### Task 1: Rewrite Makefile (ralphex pattern)

---
model: sonnet
priority: P0
complexity: Medium
---

**Description:** Replace minimal Makefile with ralphex-style build system: smart git REV, `.bin/` output, install/uninstall to `/usr/local/bin`, help target.

**Files:**
- Modify: `Makefile`

**Steps:**
- [ ] Add smart REV computation from git at the top:
  ```makefile
  TAG=$(shell git describe --tags --abbrev=0 --exact-match 2>/dev/null)
  BRANCH=$(if $(TAG),$(TAG),$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null))
  HASH=$(shell git rev-parse --short=7 HEAD 2>/dev/null)
  TIMESTAMP=$(shell git log -1 --format=%ct HEAD 2>/dev/null | xargs -I{} date -u -r {} +%Y%m%dT%H%M%S)
  GIT_REV=$(shell printf "%s-%s-%s" "$(BRANCH)" "$(HASH)" "$(TIMESTAMP)")
  REV=$(if $(filter --,$(GIT_REV)),latest,$(GIT_REV))
  ```
- [ ] Update `build` target: `cd cmd/glenv && go build -ldflags "-X main.revision=$(REV) -s -w" -o ../../.bin/glenv.$(BRANCH)` + `cp .bin/glenv.$(BRANCH) .bin/glenv`
- [ ] Update `install` target: depends on `build`, `mkdir -p /usr/local/bin`, `cp .bin/glenv /usr/local/bin/glenv`, `chmod +x`, print `✓ glenv installed` message with version
- [ ] Add `uninstall` target: `rm -f /usr/local/bin/glenv`, print `✓ glenv removed` message
- [ ] Update `test` target: keep `go test -race -coverprofile=coverage.out ./...`, add `go tool cover -func=coverage.out`
- [ ] Add `fmt` target: `gofmt -s -w` + `goimports -w` on all .go files (exclude vendor)
- [ ] Add `version` target: `@echo` branch, hash, timestamp, revision
- [ ] Add `help` target: print all available targets with descriptions
- [ ] Update `clean` target: remove `.bin/`, `dist/`, `coverage.out`, print `✓ cleaned` message
- [ ] Keep `lint`, `release`, `release-check` targets
- [ ] Update `.PHONY` with all targets including new ones
- [ ] Verify: `make version`, `make build`, `make help` work

---

### Task 2: Add `resolveVersion()` to main.go

---
model: sonnet
priority: P0
complexity: Medium
---

**Description:** Rename `version` → `revision`, add `resolveVersion()` fallback chain: ldflags → `go install` module version → VCS commit hash → `"unknown"`.

**Files:**
- Modify: `cmd/glenv/main.go`

**Steps:**
- [ ] Rename `var version = "dev"` → `var revision = "unknown"`
- [ ] Add `import "runtime/debug"`
- [ ] Implement `resolveVersion() string` with fallback chain:
  - If `revision != "unknown"` → return revision (ldflags injected)
  - Try `debug.ReadBuildInfo()` → `bi.Main.Version` if not `"(devel)"` (go install path)
  - Try VCS revision from `bi.Settings` → first 7 chars (local build without ldflags)
  - Fallback → return `"unknown"`
- [ ] Update `VersionCommand.Execute()`: use `resolveVersion()` instead of `version`
- [ ] Update `printHeader()`: use `resolveVersion()` instead of `version`
- [ ] Verify: `make build && .bin/glenv version` shows git-based revision
- [ ] Verify: `go run ./cmd/glenv version` shows VCS hash or "(devel)"

---

### Task 3: Update .goreleaser.yml (nfpms + brews)

---
model: sonnet
priority: P0
complexity: Medium
---

**Description:** Update goreleaser ldflags to inject `revision` (not `version`), add nfpms for deb/rpm packages and brews for Homebrew tap.

**Files:**
- Modify: `.goreleaser.yml`

**Steps:**
- [ ] Update ldflags from `-X main.version={{.Version}}` to `-X main.revision={{.Tag}}-{{.ShortCommit}}-{{.CommitDate}}`
- [ ] Add `nfpms` section for deb/rpm packages:
  - package_name: glenv, vendor: ohmylock
  - homepage: https://github.com/ohmylock/glenv
  - description: Fast CLI tool for managing GitLab CI/CD variables
  - license: MIT, formats: [deb, rpm]
- [ ] Add `brews` section for Homebrew tap:
  - repository: ohmylock/homebrew-tools
  - directory: Formula
  - Same homepage/description/license
- [ ] Verify: `make release-check` passes (goreleaser snapshot with nfpms)

---

### Task 4: Fix .gitignore and clean up stale files

---
model: haiku
priority: P0
complexity: Simple
---

**Description:** Fix gitignore paths for new `.bin/` directory, clean up stale build artifacts and progress files.

**Files:**
- Modify: `.gitignore`

**Steps:**
- [ ] Replace `bin/` with `.bin/` in .gitignore
- [ ] Ensure `/glenv` (root binary) is covered
- [ ] Add `coverage_no_mocks.out`
- [ ] Delete stale files: `coverage.out`, `progress*.txt`, `glenv` binary at root, `dist/`
- [ ] Verify: `git status` doesn't show build artifacts as untracked

---

### Task 5: Verification

---
model: haiku
priority: P0
complexity: Simple
---

**Steps:**
- [ ] `make version` — prints branch, hash, timestamp, revision
- [ ] `make build` — builds to `.bin/glenv` with git revision embedded
- [ ] `.bin/glenv version` — shows git-based version string (e.g. `main-a1b2c3d-20260218T120000`)
- [ ] `make install` — installs to `/usr/local/bin/glenv` with `✓` confirmation
- [ ] `make uninstall` — removes binary with `✓` confirmation
- [ ] `make help` — shows all available targets
- [ ] `make clean` — removes `.bin/`, `dist/`, coverage files with `✓` message
- [ ] `make release-check` — goreleaser snapshot passes (includes nfpms + brews sections)
- [ ] `make test` — all tests still pass

---

## File Summary

| Task | New Files | Modified Files |
|------|-----------|----------------|
| 1 | — | `Makefile` |
| 2 | — | `cmd/glenv/main.go` |
| 3 | — | `.goreleaser.yml` |
| 4 | — | `.gitignore` |
| 5 | — | — (verification only) |

## Risks and Assumptions

| Risk | Impact | Mitigation |
|------|--------|------------|
| `date -r` flag is macOS-specific | Low | Works on darwin; CI runs on Linux where `date -d @TS` is used — but Makefile is primarily for local dev |
| Homebrew tap repo doesn't exist yet | Low | goreleaser will fail on release if repo `ohmylock/homebrew-tools` is not created — create it before first release |
| nfpms requires `nfpm` binary for local testing | Low | `make release-check` uses goreleaser which bundles nfpm |

**Assumptions:**
- Project is a git repository with at least one commit
- `/usr/local/bin` is writable (may need `sudo` on some systems)
- `goimports` is installed for `make fmt` target
- Homebrew tap repo will be created at `ohmylock/homebrew-tools` before first tagged release
