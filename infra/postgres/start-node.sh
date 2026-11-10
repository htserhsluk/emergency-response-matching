#!/usr/bin/env bash
set -euo pipefail
test "${PGDATA}" = /var/lib/postgresql/data || { echo 'Unexpected PGDATA'; exit 1; }
if [ -s "$PGDATA/PG_VERSION" ]; then
  # Existing recovery/primary state is authoritative; never infer it from the node name.
  exec gosu postgres postgres -c wal_level=replica -c max_wal_senders=5 -c hot_standby=on
fi
test -z "$(ls -A "$PGDATA")" || { echo 'Nonempty uninitialized directory; recovery required'; exit 1; }
case "${BOOTSTRAP_ROLE}" in
 primary) exec /usr/local/bin/docker-entrypoint.sh postgres -c wal_level=replica -c max_wal_senders=5 ;;
 standby)
  [[ "$PRIMARY_HOST" =~ ^db-[012][ab]$ && "$NODE_ID" =~ ^node-[012][ab]$ ]] || exit 1
  chown postgres:postgres "$PGDATA"; chmod 700 "$PGDATA"
  gosu postgres pg_basebackup -d "host=$PRIMARY_HOST user=replicator application_name=$NODE_ID" -D "$PGDATA" -R -X stream
  exec gosu postgres postgres -c hot_standby=on -c wal_level=replica -c max_wal_senders=5 ;;
 *) echo 'Unknown bootstrap role'; exit 1 ;;
esac
