# glenv — Дизайн-документ реализации

**Дата:** 2026-02-17
**Статус:** Утверждён
**Module:** `github.com/ohmylock/glenv`
**Go:** 1.25

## Решения

| Аспект | Решение |
|--------|---------|
| Scope | Полный — все команды, goreleaser, CI/CD |
| Порядок | Bottom-up: пакеты → CLI → релиз |
| Concurrency | Worker Pool + Channels |
| Rate Limiter | Token bucket (`golang.org/x/time/rate`) |
| CLI Framework | `github.com/jessevdk/go-flags` |
| Config | YAML (`gopkg.in/yaml.v3`) |

## План реализации

### Слой 1: Фундамент
- `go.mod` (Go 1.25, module `github.com/ohmylock/glenv`)
- `Makefile` (build, test, lint, install, release, clean)
- `.gitignore`
- Структура директорий по спеке

### Слой 2: .env парсер (`pkg/envfile/`)
- `parser.go` — Parse KEY=VALUE
- Поддержка: single/double quotes, multiline, comments, blank lines
- Skip: placeholders (`your_`, `CHANGE_ME`, `REPLACE_WITH_`), interpolation (`${VAR}`)
- `parser_test.go` — table-driven tests

### Слой 3: Классификатор (`pkg/classifier/`)
- `classifier.go` — auto-detect masked/protected/file
- Masked: key matches secret patterns AND value >= 8 chars AND single-line
- Protected: environment == production AND key matches secret patterns
- File: key matches file patterns OR value contains PEM headers
- Custom patterns из конфига
- `classifier_test.go` — table-driven tests

### Слой 4: Конфиг (`pkg/config/`)
- `config.go` — YAML loading, defaults, env var expansion (`${VAR}`)
- Приоритет: CLI flags > config file > env vars > defaults
- Поиск: `--config` flag → `.glenv.yml` → `~/.glenv.yml`
- `config_test.go`

### Слой 5: GitLab клиент (`pkg/gitlab/`)
- `client.go` — HTTP client, auth header, rate limiter, base URL
- Rate limiter: token bucket, configurable rate/burst
- Retry: exponential backoff с jitter, respect `Retry-After` на 429
- Max 3 retries per operation
- `variables.go` — List (paginated), Get, Create, Update, Delete
- `filter[environment_scope]` для scoped operations
- `client_test.go`, `variables_test.go` — mock HTTP server

### Слой 6: Sync Engine (`pkg/sync/`)
- `engine.go` — Diff calculation + concurrent apply
- Diff: fetch current → compare → classify changes (create/update/delete/unchanged)
- Worker Pool: N горутин читают из канала задач
- Каждый воркер: rate limiter wait → API call → результат в канал
- Graceful shutdown: context cancel → drain channel → partial report
- Result collector: thread-safe сбор результатов для итогового отчёта
- `engine_test.go`

### Слой 7: CLI (`cmd/glenv/`)
- `main.go` — go-flags с subcommands
- Команды: sync, diff, list, export, delete, version
- Global flags: config, token, project, url, dry-run, debug, no-color, workers, rate-limit
- Colored output через `github.com/fatih/color`
- Graceful Ctrl+C handling

### Слой 8: Релиз
- `.goreleaser.yml` — cross-compile linux/darwin/windows (amd64/arm64)
- `.github/workflows/ci.yml` — test + lint on PR
- `.github/workflows/release.yml` — build + publish on tag
- `LICENSE` (MIT)

## Риски

| Риск | Влияние | Вероятность | Митигация |
|------|---------|-------------|-----------|
| GitLab API breaking changes | Среднее | Низкая | Pin API version, integration tests |
| Rate limit различия self-hosted | Низкое | Средняя | Configurable limits в конфиге |
| Multiline .env edge cases | Низкое | Средняя | Extensive table-driven tests |
