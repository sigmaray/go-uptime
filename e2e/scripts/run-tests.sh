#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

created_postgres_volume=false
if [[ -z "${GO_UPTIME_POSTGRES_VOLUME:-}" ]]; then
	GO_UPTIME_POSTGRES_VOLUME="$(mktemp -d "${TMPDIR:-/tmp}/go-uptime-e2e-postgres.XXXXXX")"
	export GO_UPTIME_POSTGRES_VOLUME
	created_postgres_volume=true
fi

cleanup() {
	status=$?
	trap - EXIT INT TERM
	docker compose -f ../docker-compose.with-infra.yml -p e2e-go-uptime down -v --remove-orphans 2>/dev/null || true
	if [[ "$created_postgres_volume" == true ]]; then
		if docker run --rm \
			-e "HOST_UID=$(id -u)" \
			-e "HOST_GID=$(id -g)" \
			-v "$GO_UPTIME_POSTGRES_VOLUME:/data" \
			alpine:3.21.3 sh -c 'find /data -mindepth 1 -delete && chown "$HOST_UID:$HOST_GID" /data'; then
			rmdir "$GO_UPTIME_POSTGRES_VOLUME"
		elif [[ "$status" -eq 0 ]]; then
			status=1
		fi
	fi
	exit "$status"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

./node_modules/.bin/playwright test "$@"
