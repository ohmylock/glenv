# Contributing to glenv

Thank you for your interest in contributing to glenv! This document provides guidelines and instructions for contributing.

## Development Setup

### Prerequisites

- Go 1.25 or later
- golangci-lint (for linting)
- goreleaser (for releases, optional)

### Getting Started

```bash
# Clone the repository
git clone https://github.com/ohmylock/glenv.git
cd glenv

# Install dependencies
go mod download

# Build
make build

# Run tests
make test
```

## Making Changes

### Branch Naming

- `feature/description` — new features
- `fix/description` — bug fixes
- `docs/description` — documentation changes
- `refactor/description` — code refactoring

### Code Style

- Follow standard Go conventions
- Run `make fmt` before committing
- Run `make lint` to check for issues
- Keep functions focused and small
- Write clear, descriptive variable names

### Testing

All changes should include appropriate tests:

```bash
# Run all tests with race detector
make test

# Run tests with coverage
make cover

# Run specific package tests
go test -v ./pkg/envfile/...
```

### Commit Messages

Use clear, descriptive commit messages:

```
fix: handle rate limit 429 responses correctly

- Parse Retry-After header from response
- Add exponential backoff with jitter
- Respect max retry count from config
```

Format:
- `feat:` — new feature
- `fix:` — bug fix
- `docs:` — documentation
- `test:` — tests
- `refactor:` — code refactoring
- `chore:` — maintenance tasks

## Pull Request Process

1. **Fork** the repository
2. **Create** a feature branch from `main`
3. **Make** your changes with tests
4. **Run** `make test` and `make lint`
5. **Push** to your fork
6. **Open** a Pull Request

### PR Checklist

- [ ] Tests pass (`make test`)
- [ ] Linter passes (`make lint`)
- [ ] Code is formatted (`make fmt`)
- [ ] New features are documented
- [ ] Commit messages follow conventions

## Reporting Bugs

When reporting bugs, please include:

1. glenv version (`glenv --version`)
2. Go version (`go version`)
3. Operating system and architecture
4. Steps to reproduce
5. Expected vs actual behavior
6. Relevant logs or error messages

## Feature Requests

Feature requests are welcome! Please:

1. Check existing issues for duplicates
2. Describe the use case clearly
3. Explain why it would benefit users
4. Consider implementation complexity

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you agree to uphold this code.

## Questions?

Feel free to open an issue for any questions about contributing.
