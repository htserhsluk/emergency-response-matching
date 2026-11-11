#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
set -a
source infra/compose/.env
set +a
export ERM_TEST_AMQP_URL="amqp://erm:${ERM_BROKER_PASSWORD}@127.0.0.1:5672/erm-tests"
export ERM_TEST_DATABASE_URL="postgres://erm:${ERM_DB_PASSWORD}@127.0.0.1:${1:-55432}/erm?sslmode=disable"
go test -tags=broker ./internal/broker -count=1 -v -timeout=60s
go test -race -tags=integration,broker ./tests/integration -count=1 -v -timeout=90s
