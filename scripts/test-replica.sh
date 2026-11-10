#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
set -a
source infra/compose/.env
set +a
export ERM_TEST_DATABASE_URL="postgres://erm:${ERM_DB_PASSWORD}@127.0.0.1:${1:-55432}/erm?sslmode=disable"
export ERM_TEST_STANDBY_URL="postgres://erm:${ERM_DB_PASSWORD}@127.0.0.1:${2:-55433}/erm?sslmode=disable"
go test -tags=integration,replica ./tests/integration/... -count=1 -v -timeout=60s
