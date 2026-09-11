#!/bin/sh
# Dependency-freeze gate (ADR-0002, as amended by ADR-0039):
# go.mod may require exactly golang.org/x/term and golang.org/x/sys, nothing else.
# Plus the import-graph allowlist: public packages stay stdlib-only, and every
# base64 decode uses the canonical decoder.
set -eu
cd "$(dirname "$0")/.."

mods=$(go list -m all | tail -n +2 | awk '{print $1}' | sort)
want=$(printf 'golang.org/x/sys\ngolang.org/x/term\n')
if [ "$mods" != "$want" ]; then
  echo "DEP FREEZE FAIL: go.mod modules are:" >&2
  echo "$mods" >&2
  exit 1
fi
echo "dep freeze ok: x/term + x/sys only"

# Public packages import nothing outside the stdlib and nothing from internal/.
for pkg in ./envelope ./wordlist; do
  bad=$(go list -deps "$pkg" | grep -E 'golang.org/x/|burnerpad-cli/internal' || true)
  if [ -n "$bad" ]; then
    echo "IMPORT ALLOWLIST FAIL: $pkg pulls: $bad" >&2
    exit 1
  fi
done
echo "public-package import allowlist ok"

# Every production base64 decode routes through envelope.DecodeCanonical.
bad=$(grep -rn 'base64\.' --include='*.go' . \
  | grep -v '^\./envelope/b64url\.go' \
  | grep -v '_test\.go' \
  | grep -E 'Decode' || true)
if [ -n "$bad" ]; then
  echo "BASE64 GATE FAIL: raw base64 decode outside envelope/b64url.go:" >&2
  echo "$bad" >&2
  exit 1
fi
echo "single-decode-gate ok"
