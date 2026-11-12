#!/usr/bin/env bash
# Run only in a fresh disposable container, with no database/archive mounts.
set -euo pipefail
test ! -e /target && test ! -e /archive
mkdir /target /archive /fixture-bin
printf '17\n' > /target/PG_VERSION
printf 'original\n' > /target/old-data
cat > /fixture-bin/pg_controldata <<'EOF'
#!/bin/sh
echo 'Database system identifier: 12345'
EOF
cat > /fixture-bin/gosu <<'EOF'
#!/bin/sh
mkdir -p /target/.rejoin-new
echo 17 > /target/.rejoin-new/PG_VERSION
touch /target/.rejoin-new/standby.signal
echo new > /target/.rejoin-new/new-data
EOF
cat > /fixture-bin/sync <<'EOF'
#!/bin/sh
# Emulate an unavailable unrelated filesystem: a global sync cannot finish.
if [ "$#" -eq 0 ]; then echo 'Unbounded global filesystem flush' >&2; exit 99; fi
exec /usr/bin/sync "$@"
EOF
chmod +x /fixture-bin/*
export PATH="/fixture-bin:$PATH" TARGET_ID=0a PRIMARY_ID=0b EXPECTED_SYSTEM_ID=12345
bash /src/infra/postgres/rejoin.sh
test -f /target/standby.signal && test -f /target/new-data
test ! -f /target/old-data && test ! -f /archive/0a/rejoin-state
test "$(cat /archive/0a/previous-*/old-data)" = original
echo 'PASS: recovery uses scoped durability flushes and preserves original files.'
