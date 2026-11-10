#!/usr/bin/env bash
set -euo pipefail
[[ "$NODE_ID" =~ ^[012][ab]$ ]] || exit 1
test -s /source/PG_VERSION || exit 1
mkdir -p "/archive/$NODE_ID"
shopt -s nullglob
for entry in /source/previous-*; do
  dest="/archive/$NODE_ID/$(basename "$entry")-$(date +%s%N)"
  test ! -e "$dest" || exit 1
  mv -- "$entry" "$dest"
done
