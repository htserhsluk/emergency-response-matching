#!/usr/bin/env bash
set -euo pipefail
test -n "${REPLICATION_PASSWORD:?Replication credential required}"
psql -U erm -d erm -v ON_ERROR_STOP=1 -v replication_password="$REPLICATION_PASSWORD" <<'SQL'
SELECT format('CREATE ROLE replicator WITH LOGIN REPLICATION PASSWORD %L', :'replication_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='replicator')
\gexec
SQL
if ! grep -q '^host replication replicator ' "$PGDATA/pg_hba.conf"; then
  printf '\nhost replication replicator all scram-sha-256\n' >> "$PGDATA/pg_hba.conf"
fi
psql -U erm -d erm -v ON_ERROR_STOP=1 -c 'SELECT pg_reload_conf()'
