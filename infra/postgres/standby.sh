#!/usr/bin/env bash
set -euo pipefail
test "${PGDATA}" = /var/lib/postgresql/data || { echo 'Unexpected PGDATA; refusing bootstrap'; exit 1; }
if [ ! -s "$PGDATA/PG_VERSION" ]; then
  test -z "$(ls -A "$PGDATA")" || { echo 'Nonempty uninitialized directory; manual recovery required'; exit 1; }
  chown postgres:postgres "$PGDATA"
  chmod 700 "$PGDATA"
  gosu postgres pg_basebackup -d 'host=db-0a user=replicator application_name=node-0b' -D "$PGDATA" -R -X stream
fi
test -f "$PGDATA/standby.signal" || { echo 'Standby signal missing; refusing unintended primary'; exit 1; }
exec gosu postgres postgres -c hot_standby=on
