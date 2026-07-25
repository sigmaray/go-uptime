# Go Uptime

Сервис мониторинга доступности HTTP/HTTPS URL: периодические проверки, инциденты простоя, история проверок (heartbeats) и уведомления в Telegram и по SMTP.

Админ-панель на Gin + Bootstrap. Фоновый worker проверяет мониторы и пишет результаты в PostgreSQL.

English version: [README.md](README.md)

## Возможности

- Мониторинг HTTP/HTTPS URL с настраиваемым интервалом (глобально и на монитор)
- Статус up/down, время ответа, история проверок
- Инциденты простоя с автоматическим разрешением при восстановлении
- Уведомления при смене статуса: Telegram (Shoutrrr) и email (SMTP)
- Админ-панель: дашборд, пользователи, мониторы, heartbeats, инциденты, настройки, логи
- Страница Info с метриками worker (concurrency, очередь уведомлений)
- CLI для миграций, пользователей и служебных операций с БД
- Goose-миграции (не запускаются автоматически при старте)
- E2E на Playwright, unit/integration-тесты на Go

## Требования

- Go 1.25+
- PostgreSQL 16+
- Docker и Docker Compose (опционально)
- Node.js 20+ (только для Playwright)

## Быстрый старт (локально)

1. Скопируйте конфиг и задайте секреты:

```sh
cp .env.example .env
```

Обязательные переменные: `GO_UPTIME_SESSION_SECRET`, `GO_UPTIME_DATABASE_PASSWORD`.

2. Поднимите PostgreSQL (локально или через Docker) и убедитесь, что параметры в `.env` совпадают с БД.

3. Примените миграции и создайте пользователя:

```sh
make migrate
go run . db-users-seed   # admin / admin
```

4. Запустите сервер:

```sh
make server
```

Откройте http://localhost:8080 — редирект на `/admin/`.

## Конфигурация

Конфиг читается из окружения (и из `.env` через `godotenv`). Пример: [`.env.example`](.env.example).

| Переменная | Описание | По умолчанию |
|---|---|---|
| `GO_UPTIME_ENVIRONMENT` | `development` / иное | `development` |
| `GO_UPTIME_HTTP_PORT` | HTTP-порт | `8080` |
| `GO_UPTIME_SESSION_SECRET` | Секрет cookie-сессии | обязателен |
| `GO_UPTIME_SESSION_SECURE` | Secure-флаг cookie | `false` |
| `GO_UPTIME_INCIDENT_RETENTION_DAYS` | Срок хранения закрытых инцидентов | `90` |
| `GO_UPTIME_MAX_RESOLVED_INCIDENTS_PER_MONITOR` | Лимит закрытых инцидентов на монитор | `100` |
| `GO_UPTIME_CHECK_CONCURRENCY` | Максимум параллельных HTTP-проверок | `150` |
| `GO_UPTIME_DATABASE_*` | Хост, порт, пользователь, пароль, имя БД | см. `.env.example` |
| `GO_UPTIME_ENABLE_PLAYWRIGHT_API` | Тестовый REST API для e2e | `false` |
| `GO_UPTIME_TEST_DATABASE_NAME` | Имя БД для тестов | `go-uptime-test` |
| `GIN_MODE` | Режим Gin | `release` |
| `LOG_LEVEL` | Уровень zerolog | `info` |

Уведомления (Telegram URL, SMTP) настраиваются в админке: **Settings**.

## Docker Compose

### Только приложение (внешний PostgreSQL в сети `infra`)

Нужна уже запущенная PostgreSQL в Docker-сети `infra` (например, из `~/r/sandbox/infra/postgres/`).

```sh
cp .env.example .env
# в .env задайте пароль БД и session secret
docker compose up -d --build
```

Приложение слушает `${GO_UPTIME_HTTP_PORT:-8080}`. Миграции выполните вручную:

```sh
docker compose exec go-uptime ./go-uptime db-goose-migrate
docker compose exec go-uptime ./go-uptime db-users-seed
```

### Полный стек (приложение + PostgreSQL)

Для локальной разработки, CI и e2e:

```sh
docker compose -f docker-compose.with-infra.yml up -d --build
docker compose -f docker-compose.with-infra.yml run --rm app ./go-uptime db-goose-migrate
docker compose -f docker-compose.with-infra.yml run --rm app ./go-uptime db-users-seed
```

Приложение доступно на http://localhost:18081.

## CLI

```sh
go run . server                 # HTTP-сервер + worker
go run . db-goose-migrate       # Goose-миграции (рекомендуется)
go run . db-gorm-migrate        # GORM AutoMigrate (вспомогательно)
go run . db-users-create        # создать пользователя интерактивно
go run . db-users-seed          # admin/admin
go run . db-users-show
go run . db-users-delete-all
go run . db-clear-table [table]
go run . db-clear-all-tables
go run . db-drop-table [table]
go run . db-drop-all-tables
go run . db-execute-sql "SELECT 1"
```

Алиас: `go run . s` → `server`.

## Makefile

```sh
make fmt        # gofmt
make vet
make lint-build # кастомный golangci-lint с NilAway (один раз)
make lint       # ./custom-gcl run (соберёт бинарник, если его нет)
make test
make build
make migrate
make server
```

## Тесты

### Go

Нужна отдельная тестовая БД (`GO_UPTIME_TEST_DATABASE_NAME`, по умолчанию `go-uptime-test`). Не используйте рабочую БД разработки.

```sh
make test
# или
go test ./...
```

### Playwright

```sh
cd e2e
npm ci
npx playwright install --with-deps chromium
npm test
```

Скрипт поднимает стек из `docker-compose.with-infra.yml` с включённым Playwright API и после тестов останавливает его.

## Структура проекта

```text
cmd/           CLI (Cobra)
cli/           Реализации CLI-команд
config/        Конфигурация (envconfig)
database/      Подключение к БД и миграции
handlers/      HTTP-обработчики Gin
middlewares/   Auth, логирование, rate limit
models/        GORM-модели и доступ к данным
worker/        Фоновые HTTP-проверки и уведомления
internal/      applog, notify, cliutil
server/        Запуск HTTP-сервера
migrations/    Goose SQL
templates/     HTML (admin)
static/        CSS/JS
e2e/           Playwright
```

## Безопасность

- Не коммитьте `.env` и реальные секреты.
- Смените `GO_UPTIME_SESSION_SECRET` и пароль БД перед продакшеном.
- После `db-users-seed` сразу смените пароль `admin`.
- `GO_UPTIME_ENABLE_PLAYWRIGHT_API` оставляйте выключенным вне тестов.
- Раздел **Tools** в админке доступен только при `GO_UPTIME_ENVIRONMENT=development`.

## Лицензия

Приватный / внутренний проект, если иное не указано отдельно.
