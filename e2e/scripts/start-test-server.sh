#!/usr/bin/env bash
# Поднимает тестовый сервер go-uptime в Docker Compose для e2e-тестов Playwright.
# Playwright запускает этот скрипт автоматически через опцию webServer в playwright.config.ts.
set -euo pipefail

# Корень репозитория: на два уровня выше каталога e2e/scripts/.
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

# Имя тестовой БД обязательно — Playwright передаёт GO_UPTIME_TEST_DATABASE_NAME при старте webServer.
if [[ -z "${GO_UPTIME_TEST_DATABASE_NAME:-}" ]]; then
  echo "GO_UPTIME_TEST_DATABASE_NAME is required" >&2
  exit 1
fi

# Переменные окружения: dev-режим, фиксированный секрет сессии, включён Playwright API (/api/playwright/*).
export GO_UPTIME_ENABLE_PLAYWRIGHT_API=true
export GO_UPTIME_ENVIRONMENT=development
export GO_UPTIME_SESSION_SECRET=test-session-secret
export GO_UPTIME_DATABASE_NAME="${GO_UPTIME_TEST_DATABASE_NAME}"

# Compose из docker-compose.with-infra.yml; -p e2e-go-uptime изолирует e2e от локального dev-стека.
COMPOSE="docker compose -f docker-compose.with-infra.yml -p e2e-go-uptime"

# Шаг 1: чистый старт — удаляем старые контейнеры и volumes (ошибки при первом запуске игнорируем).
$COMPOSE down -v --remove-orphans 2>/dev/null || true
# Шаг 2: собираем образ приложения.
$COMPOSE build app
# Шаг 3: поднимаем Postgres и ждём готовности (--wait опирается на healthcheck контейнера).
$COMPOSE up -d postgres --wait
# Шаг 4: одноразовый контейнер app — накатываем миграции goose на тестовую БД.
$COMPOSE run --rm -e GO_UPTIME_DATABASE_NAME="${GO_UPTIME_TEST_DATABASE_NAME}" app ./go-uptime db-goose-migrate
# Шаг 5: foreground-запуск app (exec заменяет shell — Playwright видит живой процесс и ждёт /health).
exec $COMPOSE up --build app
