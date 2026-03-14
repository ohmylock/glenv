#!/usr/bin/env bash
#
# Release script for glenv
# Loads tokens from config file and runs goreleaser
#
set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}==>${NC} $1"; }
log_warn() { echo -e "${YELLOW}Warning:${NC} $1"; }
log_error() { echo -e "${RED}Error:${NC} $1" >&2; }

# Config file locations (checked in order)
CONFIG_LOCATIONS=(
    ".release.env"
    "$HOME/.config/glenv/release.env"
    "$HOME/.glenv-release.env"
)

# Find and load config
load_config() {
    local config_file=""

    for loc in "${CONFIG_LOCATIONS[@]}"; do
        if [[ -f "$loc" ]]; then
            config_file="$loc"
            break
        fi
    done

    if [[ -z "$config_file" ]]; then
        log_error "No config file found. Create one of:"
        for loc in "${CONFIG_LOCATIONS[@]}"; do
            echo "  - $loc"
        done
        echo ""
        echo "Config file format:"
        echo "  GITHUB_TOKEN=ghp_xxxxxxxxxxxx"
        echo "  HOMEBREW_TAP_TOKEN=ghp_xxxxxxxxxxxx"
        exit 1
    fi

    log_info "Loading config from: $config_file"

    # Source the config file
    set -a
    source "$config_file"
    set +a
}

# Validate required tokens
validate_tokens() {
    local missing=0

    if [[ -z "${GITHUB_TOKEN:-}" ]]; then
        log_error "GITHUB_TOKEN is not set"
        missing=1
    fi

    if [[ -z "${HOMEBREW_TAP_TOKEN:-}" ]]; then
        log_warn "HOMEBREW_TAP_TOKEN is not set (Homebrew formula won't be updated)"
    fi

    if [[ $missing -eq 1 ]]; then
        exit 1
    fi

    log_info "Tokens loaded successfully"
}

# Check prerequisites
check_prerequisites() {
    if ! command -v goreleaser &> /dev/null; then
        log_error "goreleaser is not installed. Install with: brew install goreleaser"
        exit 1
    fi

    if ! git rev-parse --is-inside-work-tree &> /dev/null; then
        log_error "Not in a git repository"
        exit 1
    fi

    # Check for uncommitted changes
    if [[ -n "$(git status --porcelain)" ]]; then
        log_warn "You have uncommitted changes"
        read -p "Continue anyway? [y/N] " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi
}

# Get current version from tag
get_version() {
    local tag
    tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "")

    if [[ -z "$tag" ]]; then
        log_error "No git tag found. Create one with: make tag VERSION=v0.1.0"
        exit 1
    fi

    echo "$tag"
}

# Main
main() {
    local mode="${1:-}"

    echo ""
    echo "=========================================="
    echo "  glenv Release Script"
    echo "=========================================="
    echo ""

    load_config
    validate_tokens
    check_prerequisites

    local version
    version=$(get_version)
    log_info "Releasing version: $version"

    # Export tokens for goreleaser
    export GITHUB_TOKEN
    export HOMEBREW_TAP_TOKEN

    if [[ "$mode" == "--dry-run" || "$mode" == "-n" ]]; then
        log_info "Running goreleaser in snapshot mode (dry-run)..."
        goreleaser release --snapshot --clean
    else
        log_info "Running goreleaser..."
        goreleaser release --clean
    fi

    echo ""
    log_info "Release complete!"
}

main "$@"
