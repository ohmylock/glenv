# Get the latest commit branch, hash, and date
TAG=$(shell git describe --tags --abbrev=0 --exact-match 2>/dev/null)
BRANCH=$(if $(TAG),$(TAG),$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null))
HASH=$(shell git rev-parse --short=7 HEAD 2>/dev/null)
TIMESTAMP=$(shell git log -1 --format=%ct HEAD 2>/dev/null | xargs -I{} date -u -r {} +%Y%m%dT%H%M%S)
GIT_REV=$(shell printf "%s-%s-%s" "$(BRANCH)" "$(HASH)" "$(TIMESTAMP)")
REV=$(if $(filter --,$(GIT_REV)),dev,$(GIT_REV))

LDFLAGS := -ldflags "-X main.version=$(REV) -s -w"

.PHONY: all build test cover lint fmt race install uninstall release release-check clean version help

all: test build

build:
	go build $(LDFLAGS) -o bin/glenv ./cmd/glenv

test:
	go test -race ./...

cover:
	go clean -testcache
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@rm -f coverage.out

lint:
	golangci-lint run --max-issues-per-linter=0 --max-same-issues=0 2>/dev/null || go vet ./...

fmt:
	gofmt -s -w $$(find . -type f -name "*.go" -not -path "./vendor/*")
	goimports -w $$(find . -type f -name "*.go" -not -path "./vendor/*") 2>/dev/null || true

race:
	go test -race -timeout=60s ./...

version:
	@echo "branch: $(BRANCH), hash: $(HASH), timestamp: $(TIMESTAMP)"
	@echo "version: $(REV)"

install: build
	@mkdir -p /usr/local/bin
	@cp bin/glenv /usr/local/bin/glenv
	@chmod +x /usr/local/bin/glenv
	@echo "✓ glenv installed to /usr/local/bin/glenv"
	@echo "  Version: $$(glenv version 2>/dev/null || echo 'run: glenv --help')"

uninstall:
	@rm -f /usr/local/bin/glenv
	@echo "✓ glenv removed from /usr/local/bin/"

release:
	goreleaser release --clean

release-check:
	goreleaser release --snapshot --clean --skip=publish

clean:
	@rm -rf bin/ dist/ coverage.out
	@go clean -testcache
	@echo "✓ cleaned: bin/, dist/, coverage, test cache"

help:
	@echo "glenv — Sync GitLab CI/CD variables from .env files"
	@echo ""
	@echo "Build targets:"
	@echo "  make build       Compile binary to bin/glenv"
	@echo "  make install     Install binary to /usr/local/bin/"
	@echo "  make uninstall   Remove binary from /usr/local/bin/"
	@echo "  make clean       Clean build artifacts and cache"
	@echo ""
	@echo "Development targets:"
	@echo "  make test        Run tests with race detector"
	@echo "  make cover       Run tests with coverage report"
	@echo "  make race        Run tests with race detector (60s timeout)"
	@echo "  make lint        Run golangci-lint (fallback: go vet)"
	@echo "  make fmt         Format code (gofmt + goimports)"
	@echo "  make version     Show version info (branch, hash, timestamp)"
	@echo ""
	@echo "Release targets:"
	@echo "  make release       Run goreleaser"
	@echo "  make release-check Run goreleaser snapshot (no publish)"
	@echo ""
	@echo "Quick start:"
	@echo "  make             Run: make test && make build"
	@echo "  make install     Install to /usr/local/bin"
	@echo "  glenv --help     Show CLI help"
