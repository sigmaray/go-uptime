#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if [[ -z "${GO_UPTIME_TEST_DATABASE_NAME:-}" ]]; then
  echo "GO_UPTIME_TEST_DATABASE_NAME is required" >&2
  exit 1
fi

export GO_UPTIME_ENABLE_PLAYWRIGHT_API=true
export GO_UPTIME_ENVIRONMENT=development
export GO_UPTIME_SESSION_SECRET=test-session-secret
export GO_UPTIME_DATABASE_NAME="${GO_UPTIME_TEST_DATABASE_NAME}"

COMPOSE="docker compose -f docker-compose.with-infra.yml -p e2e-go-uptime"

$COMPOSE down -v --remove-orphans 2>/dev/null || true
$COMPOSE build app
$COMPOSE up -d postgres --wait
$COMPOSE run --rm -e GO_UPTIME_DATABASE_NAME="${GO_UPTIME_TEST_DATABASE_NAME}" app ./go-uptime db-goose-migrate
exec $COMPOSE up --build app
