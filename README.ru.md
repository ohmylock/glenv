<p align="right">
  <a href="README.md">🇬🇧 Read in English</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/go-1.25+-00ADD8?style=flat&logo=go" alt="Go версия 1.25 и выше">
  <a href="https://github.com/ohmylock/glenv/actions/workflows/ci.yml"><img src="https://github.com/ohmylock/glenv/actions/workflows/ci.yml/badge.svg" alt="Статус CI сборки"></a>
  <a href="https://github.com/ohmylock/glenv/releases"><img src="https://img.shields.io/github/v/release/ohmylock/glenv?include_prereleases" alt="Последний релиз"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="Лицензия MIT"></a>
  <a href="https://goreportcard.com/report/github.com/ohmylock/glenv"><img src="https://goreportcard.com/badge/github.com/ohmylock/glenv" alt="Go Report Card"></a>
</p>

<h1 align="center">glenv</h1>

<p align="center">
  <b>Синхронизация .env файлов с переменными GitLab CI/CD — массовый импорт, экспорт и управление через API</b><br>
  <i>Быстрый конкурентный CLI инструмент для управления секретами GitLab и синхронизации dotenv</i>
</p>

<p align="center">
  <a href="#быстрый-старт">Быстрый старт</a> •
  <a href="#возможности">Возможности</a> •
  <a href="#установка">Установка</a> •
  <a href="#использование">Использование</a> •
  <a href="#конфигурация">Конфигурация</a> •
  <a href="#faq">FAQ</a>
</p>

---

## Что такое glenv?

**glenv** — утилита командной строки для синхронизации `.env` файлов с переменными GitLab CI/CD. Решает проблему управления переменными окружения GitLab в промышленных масштабах — массовый импорт, экспорт, просмотр diff и синхронизация сотен переменных за секунды через GitLab API.

Вместо ручного кликания в веб-интерфейсе GitLab или написания хрупких bash-скриптов, glenv предоставляет:
- **Массовые операции** — импорт/экспорт целых `.env` файлов одной командой
- **Умная классификация** — автоматическое определение masked, protected и file-type переменных
- **Безопасный рабочий процесс** — предварительный просмотр изменений через diff перед применением
- **Конкурентная синхронизация** — параллельные API-вызовы со встроенным ограничением частоты
- **Мультиокружение** — управление production, staging и любыми окружениями через конфиг

### Почему glenv, а не альтернативы?

| Проблема | Решение glenv |
|---------|----------------|
| Веб-интерфейс GitLab медленный при большом количестве переменных | Синхронизация сотен переменных за секунды |
| Bash-скрипты хрупкие и последовательные | Конкурентные воркеры с retry и rate limiting |
| Нет предварительного просмотра изменений | Команда diff показывает создание/обновление/удаление до sync |
| Ручная классификация переменных | Автодетект masked/protected/file по шаблонам ключей |
| Разные конфиги для разных окружений | Единый YAML-конфиг для всех окружений |

**glenv** написан на Go — единый статический бинарник, нет зависимостей рантайма, работает на Linux, macOS и Windows.

## Возможности

- **Конкурентная синхронизация** — настраиваемый пул воркеров с token bucket rate limiter
- **Умная классификация** — автодетект masked, protected и file-type переменных по шаблонам ключей
- **Защита от rate limit** — соблюдает лимиты GitLab API, обрабатывает 429 через Retry-After и exponential backoff
- **Diff перед sync** — предварительный просмотр изменений (создание/обновление/удаление)
- **Dry-run режим** — видите что произойдёт без реальных API-вызовов
- **Мультиокружение** — синхронизация production, staging или любого окружения из конфига
- **Переменные проекта и группы** — синхронизация переменных на уровне проекта и на уровне группы
- **Экспорт** — загрузка текущих переменных GitLab в формат `.env` файла
- **Парсер .env** — поддержка многострочных значений, кавычек, комментариев, обнаружение плейсхолдеров
- **Zero config** — работает только с токеном и ID проекта, конфиг-файл необязателен
- **Поддержка self-hosted** — работает с любым инстансом GitLab, настраиваемые rate limits

## Быстрый старт

```bash
# Установка
go install github.com/ohmylock/glenv/cmd/glenv@latest

# Задать учётные данные (или использовать конфиг-файл)
export GITLAB_TOKEN="glpat-xxxxxxxxxxxx"
export GITLAB_PROJECT_ID="12345678"

# Синхронизировать .env файл в GitLab
glenv sync -f .env.production -e production

# Сначала предварительный просмотр (рекомендуется)
glenv diff -f .env.production -e production
```

## Установка

### Homebrew (macOS/Linux)

```bash
brew install ohmylock/tools/glenv
```

### Скачать бинарник

Скачайте подходящий бинарник для вашей платформы со страницы [releases](https://github.com/ohmylock/glenv/releases):

| Платформа | Архитектура | Файл |
|----------|--------------|------|
| macOS | Apple Silicon | `glenv_*_darwin_arm64.tar.gz` |
| macOS | Intel | `glenv_*_darwin_amd64.tar.gz` |
| Linux | x86_64 | `glenv_*_linux_amd64.tar.gz` |
| Linux | ARM64 | `glenv_*_linux_arm64.tar.gz` |
| Windows | x86_64 | `glenv_*_windows_amd64.zip` |

### Go install

```bash
go install github.com/ohmylock/glenv/cmd/glenv@latest
```

### Сборка из исходников

```bash
git clone https://github.com/ohmylock/glenv.git
cd glenv
make build
# бинарник: bin/glenv
```

## Использование

### Синхронизация переменных

Основная команда. Читает `.env` файл и синхронизирует переменные в GitLab:

```bash
# Синхронизировать один файл в конкретное окружение
glenv sync -f .env.production -e production

# Синхронизировать с dry-run (только предпросмотр)
glenv sync -f .env.staging -e staging --dry-run

# Синхронизировать все окружения из конфига
glenv sync --all

# Переопределить rate limits для self-hosted GitLab
glenv sync -f .env -e production --workers 10 --rate-limit 50
```

### Переменные на уровне группы

Синхронизируйте переменные на уровне группы GitLab вместо проекта. Переменные группы являются общими для всех проектов в группе:

```bash
# Синхронизировать в группу GitLab используя ID группы
glenv sync --group 12345 -f .env -e production

# Использовать переменную окружения вместо флага
export GITLAB_GROUP_ID=12345
glenv sync -f .env -e production

# Получить список всех переменных в группе
glenv list --group 12345

# Предварительный просмотр изменений перед синхронизацией в группу
glenv diff --group 12345 -f .env

# Экспортировать переменные из группы
glenv export --group 12345 -o group-vars.env

# Удалить переменную из группы
glenv delete --group 12345 --key SECRET_API_KEY
```

> **Примечание:** флаги `--project` и `--group` исключают друг друга. Используйте один или другой, но не оба.

### Diff (предварительный просмотр изменений)

Сравнить локальный `.env` файл с текущими переменными GitLab:

```bash
glenv diff -f .env.production -e production
```

Вывод:
```
+ DB_HOST=postgres.internal
~ API_KEY: *** → ***
- OLD_VAR
= LOG_LEVEL
```

### Список переменных

```bash
# Все переменные
glenv list

# Отфильтровать по окружению
glenv list -e production
```

### Экспорт переменных

Загрузить переменные GitLab в локальный `.env` файл:

```bash
glenv export -e production -o .env.production.backup
```

> **Примечание:** File-type переменные (сертификаты, PEM-ключи) исключаются из вывода и заменяются комментарием `# KEY (file type, skipped)`. Используйте `glenv list` для просмотра их наличия.

### Удаление переменных

```bash
# Удалить одну переменную
glenv delete -e production OLD_SECRET

# Удалить несколько переменных
glenv delete -e staging KEY1 KEY2 KEY3 --force
```

## Конфигурация

glenv работает без конфига (только через переменные окружения), но конфиг-файл открывает возможности мультиокружения.

**Порядок поиска конфиг-файла:**
1. Путь из флага `--config`
2. `.glenv.yml` в текущей директории
3. `~/.glenv.yml`

```yaml
# Подключение к GitLab
gitlab:
  url: https://gitlab.com                     # self-hosted: https://gitlab.company.com
  token: ${GITLAB_TOKEN}                      # поддерживается раскрытие env-переменных
  project_id: "12345678"                      # ID проекта (исключает друг друга с group_id)
  # group_id: "9876"                          # ID группы (для переменных уровня группы, используйте вместо project_id)

# Rate limiting (безопасные значения по умолчанию для gitlab.com)
rate_limit:
  requests_per_second: 10                     # макс. запросов API/сек (gitlab.com: ~33)
  max_concurrent: 5                           # параллельные воркеры
  retry_max: 3                                # повторов при ошибке
  retry_initial_backoff: 1s                   # задержка перед первой повторной попыткой

# Окружения
environments:
  production:
    file: deploy/gitlab-envs/.env.production
  staging:
    file: deploy/gitlab-envs/.env.staging

# Кастомные правила классификации (расширяют встроенные)
classify:
  masked_patterns:                            # ключи с этими шаблонами → masked
    - "_TOKEN"
    - "SECRET"
    - "PASSWORD"
    - "API_KEY"
    - "DSN"
  masked_exclude:                             # исключения (НЕ masked)
    - "MAX_TOKENS"
    - "TIMEOUT"
    - "PORT"
  file_patterns:                              # ключи с этими шаблонами → file type
    - "PRIVATE_KEY"
    - "_CERT"
    - "_PEM"
  file_exclude:                               # исключения (НЕ file type)
    - "_PATH"
    - "_DIR"
    - "_URL"
```

### Переменные окружения

| Переменная | Описание |
|----------|-------------|
| `GITLAB_TOKEN` | GitLab Personal Access Token (scope: `api`) |
| `GITLAB_PROJECT_ID` | ID проекта или URL-encoded путь (исключает друг друга с `GITLAB_GROUP_ID`) |
| `GITLAB_GROUP_ID` | ID группы (исключает друг друга с `GITLAB_PROJECT_ID`) |
| `GITLAB_URL` | URL инстанса GitLab (по умолчанию: `https://gitlab.com`) |
| `NO_COLOR` | Отключить цветной вывод (стандартная конвенция) |

Переменные окружения имеют приоритет над конфиг-файлом. CLI-флаги имеют наивысший приоритет.

## Как это работает

### Процесс синхронизации

1. **Parse** — читает `.env` файл, обрабатывает многострочные значения, пропускает плейсхолдеры
2. **Classify** — автодетект masked/protected/file свойств по шаблонам ключей и значений
3. **Fetch** — получает текущие переменные из GitLab (paginated, concurrent-safe)
4. **Diff** — вычисляет создания, обновления, удаления и неизменённые
5. **Apply** — распределяет изменения по пулу воркеров с rate limiting
6. **Report** — цветная сводка с таймингом и статистикой API-вызовов

### Классификация переменных

glenv автоматически определяет свойства переменных:

| Свойство | Условие |
|----------|-----------|
| **masked** | Ключ совпадает с секретным шаблоном (`_TOKEN`, `SECRET`, `PASSWORD`, ...) И значение >= 8 символов И однострочное И содержит только `[a-zA-Z0-9_:@-.+~=/]` |
| **protected** | Окружение `production` И ключ совпадает с секретным шаблоном |
| **file type** | Ключ совпадает с шаблоном файла (`PRIVATE_KEY`, `_CERT`, `_PEM`) ИЛИ значение содержит PEM-заголовки (`-----BEGIN`) |

Переменные с плейсхолдерами (`your_`, `CHANGE_ME`, `REPLACE_WITH_`) пропускаются.
Переменные с интерполяцией (`${VAR}`) пропускаются.

### Rate Limiting

glenv использует token bucket rate limiter для соблюдения лимитов GitLab API:

| Инстанс GitLab | Лимит | Конфиг по умолчанию |
|----------------|-------|----------------|
| gitlab.com | 2 000 запросов/мин (~33/сек) | 10 запросов/сек, 5 воркеров |
| Self-hosted | Настраивается | Задаётся через конфиг |

При получении 429:
1. Парсит заголовок `Retry-After`
2. Ждёт указанное время
3. Повторяет с exponential backoff + jitter
4. Максимум 3 повторных попытки (настраивается)

### Формат .env файла

Поддерживаемый синтаксис:

```bash
# Комментарии пропускаются
KEY=value
QUOTED="значение с пробелами"
SINGLE_QUOTED='значение'

# Многострочные значения
PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA...
-----END RSA PRIVATE KEY-----"

# Автоматически пропускаются:
PLACEHOLDER=your_api_key_here       # плейсхолдер обнаружен
INTERPOLATED=${OTHER_VAR}/path      # интерполяция обнаружена
```

## Справочник опций

### Глобальные опции

| Флаг | Короткий | Env | Описание | По умолчанию |
|------|-------|---------|-------------|---------|
| `--config` | `-c` | | Путь к конфиг-файлу | `.glenv.yml` |
| `--token` | | `GITLAB_TOKEN` | Токен доступа GitLab | |
| `--project` | | `GITLAB_PROJECT_ID` | ID проекта (исключает друг друга с `--group`) | |
| `--group` | | `GITLAB_GROUP_ID` | ID группы (исключает друг друга с `--project`) | |
| `--url` | | `GITLAB_URL` | URL GitLab | `https://gitlab.com` |
| `--dry-run` | `-n` | | Режим предпросмотра | `false` |
| `--no-color` | | `NO_COLOR` | Отключить цвета | `false` |
| `--workers` | `-w` | | Конкурентные воркеры | `5` |
| `--rate-limit` | | | Макс. запросов/сек | `10` |

### Опции sync

| Флаг | Короткий | Описание |
|------|-------|-------------|
| `--file` | `-f` | Путь к .env файлу |
| `--environment` | `-e` | Область окружения GitLab |
| `--all` | `-a` | Синхронизировать все окружения из конфига |
| `--delete-missing` | | Удалить переменные, отсутствующие в .env файле |
| `--no-auto-classify` | | Отключить умную классификацию |
| `--force` | | Пропустить запросы подтверждения |

### Опции export

| Флаг | Короткий | Описание |
|------|-------|-------------|
| `--environment` | `-e` | Окружение для экспорта |
| `--output` | `-o` | Путь к выходному файлу (по умолчанию: stdout) |

## Примеры

### Базовый: одно окружение

```bash
export GITLAB_TOKEN="glpat-xxxxxxxxxxxx"
export GITLAB_PROJECT_ID="12345678"

# Предварительный просмотр изменений
glenv diff -f .env.production -e production

# Применить изменения
glenv sync -f .env.production -e production
```

### Конфиг: несколько окружений

`.glenv.yml`:
```yaml
gitlab:
  token: ${GITLAB_TOKEN}
  project_id: "12345678"

environments:
  staging:
    file: deploy/.env.staging
  production:
    file: deploy/.env.production
    protected: true
```

```bash
# Синхронизировать все окружения
glenv sync --all

# Синхронизировать конкретное окружение
glenv sync -e production
```

### Self-hosted GitLab с повышенными лимитами

```yaml
gitlab:
  url: https://gitlab.company.com
  token: ${GITLAB_TOKEN}
  project_id: "42"

rate_limit:
  requests_per_second: 50
  max_concurrent: 10
```

### Интеграция в CI/CD пайплайн

```yaml
# .gitlab-ci.yml
sync-variables:
  image: golang:1.23-alpine
  script:
    - go install github.com/ohmylock/glenv/cmd/glenv@latest
    - glenv sync -f deploy/.env.${CI_ENVIRONMENT_NAME} -e ${CI_ENVIRONMENT_NAME}
  variables:
    GITLAB_TOKEN: ${DEPLOY_TOKEN}
    GITLAB_PROJECT_ID: ${CI_PROJECT_ID}
```

## Релиз

glenv использует [goreleaser](https://goreleaser.com/) для кросс-платформенных релизов.

### Релиз одной командой

```bash
# Тегировать и выпустить
git tag v1.0.0
git push origin v1.0.0
# GitHub Actions сделает всё остальное

# Или выпустить локально
make release
```

### Цели Makefile

```bash
make build          # скомпилировать бинарник в bin/glenv
make test           # запустить тесты с race detector и coverage
make lint           # запустить golangci-lint (fallback: go vet)
make install        # установить в /usr/local/bin
make release        # goreleaser release
make release-check  # goreleaser dry-run (без публикации)
make clean          # удалить артефакты сборки
```

## Разработка

### Требования

- Go 1.25+
- golangci-lint (для `make lint`)
- goreleaser (для `make release`)

### Запуск тестов

```bash
make test           # юнит-тесты с coverage
make lint           # статический анализ
go test -race ./... # race detector
```

### Интеграционные тесты

Установите `GITLAB_TOKEN` и `GITLAB_TEST_PROJECT_ID` для запуска тестов против реального инстанса GitLab:

```bash
GITLAB_TOKEN=glpat-xxx GITLAB_TEST_PROJECT_ID=123 go test -tags=integration ./...
```

## FAQ

<details>
<summary><b>Безопасно ли использовать несколько воркеров?</b></summary>

Да. glenv использует token bucket rate limiter, который контролирует общую частоту запросов к API вне зависимости от числа воркеров. Воркеры разделяют rate limiter, поэтому увеличение их числа не увеличивает частоту API-запросов — только распределяет работу эффективнее. Значение по умолчанию 10 запросов/сек значительно ниже лимита gitlab.com в 2 000 запросов/мин.
</details>

<details>
<summary><b>Что происходит при rate-limit от GitLab?</b></summary>

glenv автоматически обрабатывает ответы 429. Читает заголовок `Retry-After`, ждёт указанное время, затем повторяет с exponential backoff. После 3 неудачных попыток (настраивается) операция помечается как ошибочная и включается в итоговый отчёт.
</details>

<details>
<summary><b>Работает ли с self-hosted GitLab?</b></summary>

Да. Укажите `gitlab.url` в конфиг-файле или используйте флаг `--url`. Self-hosted инстансы могут иметь другие (или отсутствующие) rate limits — настройте `rate_limit.requests_per_second` соответственно.
</details>

<details>
<summary><b>Как обрабатываются masked переменные?</b></summary>

GitLab требует, чтобы masked переменные содержали не менее 8 символов и были однострочными. glenv автоматически определяет, какие переменные должны быть masked, на основе шаблонов имён ключей (TOKEN, SECRET, PASSWORD и т.д.) и проверяет, что значение соответствует требованиям GitLab. Шаблоны можно настроить через конфиг.
</details>

<details>
<summary><b>Можно ли синхронизировать несколько проектов?</b></summary>

В текущей версии glenv синхронизирует один проект за вызов. Для нескольких проектов используйте отдельные конфиг-файлы:
```bash
glenv sync --config project-a.yml --all
glenv sync --config project-b.yml --all
```
</details>

<details>
<summary><b>Какие форматы .env поддерживаются?</b></summary>

Стандартный формат `.env`: `KEY=VALUE`, с поддержкой одинарных/двойных кавычек, многострочных значений в кавычках и комментариев. Плейсхолдеры (`your_`, `CHANGE_ME`) и интерполяция переменных (`${VAR}`) обнаруживаются и пропускаются. Подробнее в разделе [Формат .env файла](#формат-env-файла).
</details>

## Альтернативы

| Инструмент | Язык | Возможности |
|------|----------|----------|
| **glenv** | Go | Конкурентная синхронизация, автоклассификация, rate limiting, diff preview |
| [GlabEnv](https://github.com/arisnacg/nodejs-glabenv) | Node.js | Базовая синхронизация/экспорт |
| [gitlab-dotenv](https://github.com/apicore-engineering/gitlab-dotenv) | Python | Управление переменными |
| [glab variable](https://docs.gitlab.com/cli/variable/) | Go | Официальный CLI (операции с одной переменной) |

## Вклад в проект

Вклад в проект приветствуется! Прочитайте [руководство по участию](CONTRIBUTING.md) перед отправкой PR.

## Лицензия

Лицензия MIT — см. файл [LICENSE](LICENSE).

---

<p align="center">
  Сделано с ❤️ для DevOps-сообщества
</p>
