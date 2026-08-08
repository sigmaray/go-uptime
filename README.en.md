# Go Uptime

HTTP/HTTPS uptime monitoring: periodic checks, downtime incidents, check history (heartbeats), and alerts via Telegram and SMTP.

Admin UI built with Gin and Bootstrap. A background worker probes monitors and stores results in PostgreSQL.

Русская версия: [README.md](README.md)

## Features

- HTTP/HTTPS URL monitoring with configurable intervals (global and per monitor)
- Up/down status, response time, and check history
- Downtime incidents that resolve automatically when the target recovers
- Status-change notifications: Telegram (Shoutrrr) and email (SMTP)
- Admin panel: dashboard, users, monitors, heartbeats, incidents, settings, logs
- Info page with live worker metrics (concurrency, notify queue)
- CLI for migrations, users, and database maintenance
- Goose migrations (not applied automatically on startup)
- Playwright e2e tests and Go unit/integration tests

## Requirements

- Go 1.25+
- PostgreSQL 16+
- Docker and Docker Compose (optional)
- Node.js 20+ (Playwright only)

## Quick start (local)

1. Copy the example env file and set secrets:

```sh
cp .env.example .env
```

Required: `GO_UPTIME_SESSION_SECRET`, `GO_UPTIME_DATABASE_PASSWORD`.

2. Start PostgreSQL (local or Docker) and align `.env` with the database.

3. Apply migrations and create a user:

```sh
make migrate
go run . db-users-seed   # admin / admin
```

4. Start the server:

```sh
make server
```

Open http://localhost:8080 — it redirects to `/admin/`.

## Configuration

Settings are loaded from the environment (and from `.env` via `godotenv`). See [`.env.example`](.env.example).

| Variable | Description | Default |
|---|---|---|
| `GO_UPTIME_ENVIRONMENT` | `development` / other | `development` |
| `GO_UPTIME_HTTP_PORT` | HTTP port | `8080` |
| `GO_UPTIME_SESSION_SECRET` | Cookie session secret | required |
| `GO_UPTIME_SESSION_SECURE` | Secure cookie flag (defaults to `true` outside development if unset) | `false` in `.env.example` |
| `GO_UPTIME_INCIDENT_RETENTION_DAYS` | How long resolved incidents are kept | `90` |
| `GO_UPTIME_MAX_RESOLVED_INCIDENTS_PER_MONITOR` | Cap of resolved incidents per monitor | `100` |
| `GO_UPTIME_CHECK_CONCURRENCY` | Max concurrent HTTP checks | `150` |
| `GO_UPTIME_DATABASE_*` | Host, port, user, password, database name, sslmode | see `.env.example` |
| `GO_UPTIME_ENABLE_PLAYWRIGHT_API` | Test REST API for e2e (development only) | `false` |
| `GO_UPTIME_TEST_DATABASE_NAME` | Database name for tests | `go-uptime-test` |
| `GIN_MODE` | Gin mode | `release` |
| `LOG_LEVEL` | zerolog level | `info` |

Notification channels (Telegram URL, SMTP) are configured in the admin UI under **Settings**.

## Docker Compose

### App only (external PostgreSQL on the `infra` network)

Requires PostgreSQL already running on the Docker network `infra` (for example from `~/r/sandbox/infra/postgres/`).

```sh
cp .env.example .env
# set DB password and session secret in .env
docker compose up -d --build
```

The app listens on `${GO_UPTIME_HTTP_PORT:-8080}`. Run migrations manually:

```sh
docker compose exec go-uptime ./go-uptime db-goose-migrate
docker compose exec go-uptime ./go-uptime db-users-seed
```

### Full stack (app + PostgreSQL)

For local development, CI, and e2e:

```sh
docker compose -f docker-compose.with-infra.yml up -d --build
docker compose -f docker-compose.with-infra.yml run --rm app ./go-uptime db-goose-migrate
docker compose -f docker-compose.with-infra.yml run --rm app ./go-uptime db-users-seed
```

The app is available at http://localhost:18081.

## CLI

```sh
go run . server                 # HTTP server + worker
go run . db-goose-migrate       # Goose migrations (preferred)
go run . db-gorm-migrate        # GORM AutoMigrate (helper)
go run . db-users-create        # create a user interactively
go run . db-users-seed          # admin/admin
go run . db-users-show
go run . db-users-delete-all
go run . db-clear-table [table]
go run . db-clear-all-tables
go run . db-drop-table [table]
go run . db-drop-all-tables
go run . db-execute-sql "SELECT 1"
```

Alias: `go run . s` → `server`.

## Makefile

```sh
make fmt        # gofmt
make vet
make lint-build # custom golangci-lint with NilAway (once)
make lint       # ./custom-gcl run (builds binary if missing)
make test
make build
make migrate
make server
```

## Tests

### Go

Use a separate test database (`GO_UPTIME_TEST_DATABASE_NAME`, default `go-uptime-test`). Do not point tests at the development database.

```sh
make test
# or
go test ./...
```

### Playwright

```sh
cd e2e
npm ci
npx playwright install --with-deps chromium
npm test
```

The script starts the stack from `docker-compose.with-infra.yml` with the Playwright API enabled and tears it down afterward.

## Project layout

```text
cmd/           CLI (Cobra)
cli/           CLI command implementations
config/        Configuration (envconfig)
database/      DB connection and migrations
handlers/      Gin HTTP handlers
middlewares/   Auth, logging, rate limit
models/        GORM models and data access
worker/        Background HTTP checks and notifications
internal/      applog, notify, cliutil
server/        HTTP server bootstrap
migrations/    Goose SQL
templates/     HTML (admin)
static/        CSS/JS
e2e/           Playwright
```

## Security

- Do not commit `.env` or real secrets.
- Change `GO_UPTIME_SESSION_SECRET` and the database password before production.
- After `db-users-seed`, change the `admin` password immediately.
- Keep `GO_UPTIME_ENABLE_PLAYWRIGHT_API` disabled outside tests; it is refused unless `GO_UPTIME_ENVIRONMENT=development`.
- Behind HTTPS set `GO_UPTIME_SESSION_SECURE=true` (or leave it unset outside development so it defaults on).
- Prefer `GO_UPTIME_DATABASE_SSLMODE=require` (or stricter) when PostgreSQL is not on a private network.
- The admin **Tools** section is available only when `GO_UPTIME_ENVIRONMENT=development`.

## License

Private / internal project unless stated otherwise.
