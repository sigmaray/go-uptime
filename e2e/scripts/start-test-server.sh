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
export GO_UPTIME_SESSION_SECURE=false
export GO_UPTIME_DATABASE_USER=go_uptime_test
export GO_UPTIME_DATABASE_PASSWORD=e2e-test-database-password
export GO_UPTIME_DATABASE_NAME="${GO_UPTIME_TEST_DATABASE_NAME}"
export GO_UPTIME_HTTP_PORT=18081

COMPOSE="docker compose -f docker-compose.with-infra.yml -p e2e-go-uptime"
compose_pid=""
created_postgres_volume=false

if [[ -z "${GO_UPTIME_POSTGRES_VOLUME:-}" ]]; then
	GO_UPTIME_POSTGRES_VOLUME="$(mktemp -d "${TMPDIR:-/tmp}/go-uptime-e2e-postgres.XXXXXX")"
	export GO_UPTIME_POSTGRES_VOLUME
	created_postgres_volume=true
fi

cleanup() {
	status=$?
	trap - EXIT INT TERM
	if [[ -n "$compose_pid" ]]; then
		kill "$compose_pid" 2>/dev/null || true
		wait "$compose_pid" 2>/dev/null || true
	fi
	$COMPOSE down -v --remove-orphans 2>/dev/null || true
	if [[ "$created_postgres_volume" == true ]]; then
		rm -rf -- "$GO_UPTIME_POSTGRES_VOLUME"
	fi
	exit "$status"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

$COMPOSE down -v --remove-orphans 2>/dev/null || true
$COMPOSE build app
$COMPOSE up -d postgres --wait
$COMPOSE run --rm -e GO_UPTIME_DATABASE_NAME="${GO_UPTIME_TEST_DATABASE_NAME}" app ./go-uptime db-goose-migrate
$COMPOSE up --build app &
compose_pid=$!
wait "$compose_pid"
