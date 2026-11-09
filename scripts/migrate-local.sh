#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
set -a
source infra/compose/.env
set +a
export GOOSE_DRIVER=postgres
export GOOSE_DBSTRING="postgres://erm:${ERM_DB_PASSWORD}@127.0.0.1:55432/erm?sslmode=disable"
goose -dir migrations up
