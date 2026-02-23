# Usage Examples

## Basic Workflows

### First-Time Setup

```bash
# 1. Install glenv
go install github.com/ohmylock/glenv/cmd/glenv@latest

# 2. Create a minimal config
cat > .glenv.yml << 'EOF'
gitlab:
  token: ${GITLAB_TOKEN}
  project_id: "12345678"
EOF

# 3. Set your token
export GITLAB_TOKEN="glpat-xxxxxxxxxxxx"

# 4. Preview what will be synced
glenv diff -f .env.production -e production

# 5. Sync
glenv sync -f .env.production -e production
```

### Multi-Environment Project

Config file `.glenv.yml`:

```yaml
gitlab:
  token: ${GITLAB_TOKEN}
  project_id: "12345678"

environments:
  staging:
    file: deploy/.env.staging
    protected: false
  production:
    file: deploy/.env.production
    protected: true
```

```bash
# Sync everything
glenv sync --all

# Sync only staging
glenv sync -e staging

# Check diff for production before syncing
glenv diff -e production
glenv sync -e production
```

### Without Config File

```bash
# Everything via flags and env vars
export GITLAB_TOKEN="glpat-xxxxxxxxxxxx"

glenv sync \
  --project 12345678 \
  --url https://gitlab.company.com \
  -f .env.production \
  -e production
```

## Common Scenarios

### Initial Import of All Variables

```bash
# First time: create all variables
glenv sync -f .env.staging -e staging
glenv sync -f .env.production -e production
```

### Update Changed Variables Only

```bash
# glenv automatically detects which vars changed
# Only changed variables generate API calls
glenv sync -f .env.production -e production

# Output:
#   = DB_HOST                (unchanged)
#   = DB_PORT                (unchanged)
#   ↻ API_KEY                (updated) [masked]
#   ✓ NEW_FEATURE_FLAG       (created)
```

### Backup Before Making Changes

```bash
# Export current state
glenv export -e production -o .env.production.backup

# Make changes
glenv sync -f .env.production -e production

# If something goes wrong, restore from backup
glenv sync -f .env.production.backup -e production
```

### Dry-Run for Safety

```bash
# See exactly what would happen without making changes
glenv sync -f .env.production -e production --dry-run

# Output shows creates, updates, deletes without executing them
```

### Clean Up Old Variables

```bash
# Delete variables that exist in GitLab but not in your .env file
glenv sync -f .env.production -e production --delete-missing

# This will prompt for confirmation before deleting
# Use --force to skip confirmation
```

### List Current Variables

```bash
# All variables
glenv list

# Specific environment
glenv list -e production

# Output:
#   SCOPE        KEY                          TYPE      MASKED
#   production   DB_HOST                      env_var   false
#   production   DB_PASSWORD                  env_var   true
#   production   SSH_PRIVATE_KEY              file      false
#   staging      DB_HOST                      env_var   false
```

## Self-Hosted GitLab

### Higher Rate Limits

Self-hosted GitLab may have no rate limits or much higher ones:

```yaml
gitlab:
  url: https://gitlab.company.com
  token: ${GITLAB_TOKEN}
  project_id: "42"

rate_limit:
  requests_per_second: 100    # much higher for self-hosted
  max_concurrent: 20          # more workers
```

### Behind Corporate Proxy

Set standard proxy environment variables:

```bash
export HTTPS_PROXY=http://proxy.company.com:8080
export NO_PROXY=localhost,127.0.0.1

glenv sync -f .env -e production
```

## CI/CD Integration

### GitLab CI Pipeline

```yaml
# .gitlab-ci.yml
stages:
  - sync-vars

sync-variables:
  stage: sync-vars
  image: golang:1.23-alpine
  before_script:
    - go install github.com/ohmylock/glenv/cmd/glenv@latest
  script:
    - glenv sync -f deploy/.env.${CI_ENVIRONMENT_NAME} -e ${CI_ENVIRONMENT_NAME}
  variables:
    GITLAB_TOKEN: ${DEPLOY_TOKEN}
    GITLAB_PROJECT_ID: ${CI_PROJECT_ID}
  rules:
    - if: $CI_COMMIT_BRANCH == "main"
      changes:
        - deploy/.env.*
  environment:
    name: $CI_ENVIRONMENT_NAME
```

### GitHub Actions

```yaml
# .github/workflows/sync-vars.yml
name: Sync GitLab Variables
on:
  push:
    branches: [main]
    paths: ['deploy/.env.*']

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: go install github.com/ohmylock/glenv/cmd/glenv@latest
      - run: glenv sync --all
        env:
          GITLAB_TOKEN: ${{ secrets.GITLAB_TOKEN }}
          GITLAB_PROJECT_ID: ${{ secrets.GITLAB_PROJECT_ID }}
```

## Scripting

### Sync Multiple Projects

```bash
#!/bin/bash
# sync-all-projects.sh

projects=(
  "project-a:12345:.env.production:production"
  "project-b:67890:.env.staging:staging"
  "project-c:11111:.env.production:production"
)

for entry in "${projects[@]}"; do
  IFS=':' read -r name project_id env_file environment <<< "$entry"
  echo "Syncing $name..."
  glenv sync \
    --project "$project_id" \
    -f "$env_file" \
    -e "$environment"
done
```

### Export All Environments

```bash
#!/bin/bash
# export-all.sh

for env in staging production; do
  glenv export -e "$env" -o "backup/.env.${env}.$(date +%Y%m%d)"
done
```

## .env File Examples

### Standard Variables

```bash
# Application settings
APP_NAME=myapp
APP_PORT=8080
LOG_LEVEL=INFO
DEBUG=false

# Database
DB_HOST=postgres.internal
DB_PORT=5432
DB_NAME=myapp_production
DB_PASSWORD="super-secret-password-here"

# External APIs
OPENAI_API_KEY="sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
STRIPE_SECRET_KEY="sk_test_your_stripe_key_here"
```

### Multiline Values (SSH Keys, Certificates)

```bash
# SSH deploy key (will be detected as file-type variable)
SSH_PRIVATE_KEY="-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAA
...
-----END OPENSSH PRIVATE KEY-----"

# CA certificate
CA_CERT="-----BEGIN CERTIFICATE-----
MIIFazCCA1OgAwIBAgIRAIIQz7DSQONZRGPgu2OCiwAwDQYJKoZIhvcN
...
-----END CERTIFICATE-----"
```

### Variables That Get Skipped

```bash
# Placeholders (skipped - contain "your_", "CHANGE_ME", etc.)
API_KEY=your_api_key_here
SECRET=CHANGE_ME
TOKEN=REPLACE_WITH_REAL_TOKEN

# Interpolation (skipped - contains ${VAR})
DATABASE_URL=${DB_HOST}:${DB_PORT}/${DB_NAME}
```
