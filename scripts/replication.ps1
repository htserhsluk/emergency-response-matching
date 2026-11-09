$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
docker compose --env-file infra/compose/.env -f infra/compose/compose.yaml exec -T db-0a bash /config/replication-primary.sh
if ($LASTEXITCODE) { throw 'Replication role setup failed' }
docker compose --env-file infra/compose/.env -f infra/compose/compose.yaml --profile replica up -d --wait db-0b node-0b
if ($LASTEXITCODE) { throw 'Standby startup failed' }
docker compose --env-file infra/compose/.env -f infra/compose/compose.yaml exec -T db-0a psql -U erm -d erm -v ON_ERROR_STOP=1 -c "ALTER SYSTEM SET synchronous_standby_names = 'FIRST 1 (`"node-0b`")'"
if ($LASTEXITCODE) { throw 'Synchronous standby configuration failed' }
docker compose --env-file infra/compose/.env -f infra/compose/compose.yaml exec -T db-0a psql -U erm -d erm -v ON_ERROR_STOP=1 -c 'ALTER SYSTEM SET synchronous_commit = remote_apply'
if ($LASTEXITCODE) { throw 'Synchronous commit configuration failed' }
docker compose --env-file infra/compose/.env -f infra/compose/compose.yaml exec -T db-0a psql -U erm -d erm -c 'SELECT pg_reload_conf()'
