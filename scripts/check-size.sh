#!/bin/sh
# CI fence (ADR-0017): any target binary > 9 MiB fails the build.
# The gate exists to catch import-graph drift, paired with the go list -deps allowlist.
set -eu
LIMIT=9437184 # 9 MiB
fail=0
for f in "$@"; do
  sz=$(wc -c < "$f")
  if [ "$sz" -gt "$LIMIT" ]; then
    echo "SIZE GATE FAIL: $f is $sz bytes (> $LIMIT)" >&2
    fail=1
  else
    echo "size ok: $f ($sz bytes)"
  fi
done
exit $fail
