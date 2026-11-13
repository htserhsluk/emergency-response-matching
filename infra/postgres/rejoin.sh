#!/usr/bin/env bash
set -euo pipefail
target=/target
[[ "$TARGET_ID" =~ ^[012][ab]$ && "$PRIMARY_ID" =~ ^[012][ab]$ ]] || exit 1
test "${TARGET_ID:0:1}" = "${PRIMARY_ID:0:1}" && test "$TARGET_ID" != "$PRIMARY_ID" || { echo 'Wrong shard or current primary target'; exit 1; }
mkdir -p "/archive/$TARGET_ID"
state="/archive/$TARGET_ID/rejoin-state"
incoming="$target/.rejoin-new"
if [ ! -f "$state" ]; then
 test -s "$target/PG_VERSION" || { echo 'Expected existing fenced node volume'; exit 1; }
 actual=$(pg_controldata "$target" | sed -n 's/^Database system identifier: *//p')
 test "$actual" = "$EXPECTED_SYSTEM_ID" || { echo 'Target cluster identity differs'; exit 1; }
 # Preserve an interrupted base backup before creating a fresh one.
 if [ -e "$incoming" ]; then mv -- "$incoming" "/archive/$TARGET_ID/incomplete-$(date +%s%N)"; fi
 mkdir "$incoming";chown postgres:postgres "$incoming";chmod 700 "$incoming"
 gosu postgres pg_basebackup -d "host=db-$PRIMARY_ID user=replicator application_name=node-$TARGET_ID" -D "$incoming" -R -X stream
 test -s "$incoming/PG_VERSION" && test -f "$incoming/standby.signal" || exit 1
 printf '%s %s %s ARCHIVE\n' "$PRIMARY_ID" "$EXPECTED_SYSTEM_ID" "$(date +%s%N)" > "$state.tmp"
 # pg_basebackup fsyncs its files. Flush only our changed directory entries
 # and journal; a global sync can hang on unrelated host-mounted filesystems.
 sync "$state.tmp" "$target" "/archive/$TARGET_ID"
 mv "$state.tmp" "$state"; sync "/archive/$TARGET_ID"
fi
read -r source system stamp phase < "$state"
test "$source" = "$PRIMARY_ID" && test "$system" = "$EXPECTED_SYSTEM_ID" || { echo 'Recovery journal source differs'; exit 1; }
backup="/archive/$TARGET_ID/previous-$stamp"
mkdir -p "$backup"
shopt -s dotglob nullglob
if [ "$phase" = ARCHIVE ]; then
 for entry in "$target"/*; do
  test "$entry" = "$incoming" && continue
  mv -- "$entry" "$backup/"
 done
 printf '%s %s %s INSTALL\n' "$source" "$system" "$stamp" > "$state.tmp"
 sync "$target" "$backup" "$state.tmp"
 mv "$state.tmp" "$state"; sync "/archive/$TARGET_ID"
fi
for entry in "$incoming"/*; do mv -- "$entry" "$target/"; done
test -s "$target/PG_VERSION" && test -f "$target/standby.signal" || exit 1
if [ -d "$incoming" ]; then rmdir "$incoming"; fi
chown postgres:postgres "$target";chmod 700 "$target"
sync "$target" "$backup"
rm -- "$state"; sync "/archive/$TARGET_ID"
echo 'Rebuilt standby; previous database files preserved.'
