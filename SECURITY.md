# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability in glenv, please report it responsibly:

1. **Do NOT** open a public GitHub issue
2. Email the maintainers directly or use GitHub's private vulnerability reporting feature
3. Include details about the vulnerability and steps to reproduce

We will respond within 48 hours and work with you to understand and resolve the issue.

## Security Best Practices

When using glenv:

- Store `GITLAB_TOKEN` securely (use environment variables, not config files)
- Use tokens with minimal required scopes (`api` scope for full functionality)
- Review `.glenv.yml` before committing (it's in `.gitignore` by default)
- Use `--dry-run` to preview changes before applying
