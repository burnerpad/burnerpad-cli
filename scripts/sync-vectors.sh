#!/bin/sh
# spec-drift check: byte-compare the vendored language-neutral SPEC, vectors,
# and wordlist against the upstream checkout (default: the sibling burnerpad-lite).
# Usage: scripts/sync-vectors.sh [path-to-burnerpad-lite]
set -eu
cd "$(dirname "$0")/.."
LITE="${1:-../burnerpad-lite}"
CRYPTO="$LITE/priv/static/vendor/crypto-js"

fail=0
cmp -s "$CRYPTO/vectors/v1.json" envelope/testdata/v1.json || { echo "DRIFT: vectors/v1.json differs" >&2; fail=1; }
cmp -s "$CRYPTO/SPEC.md" spec/SPEC.md || { echo "DRIFT: SPEC.md differs" >&2; fail=1; }

# Wordlist: regenerate from the web driver's array and compare in memory.
node -e '
const fs = require("fs");
const src = fs.readFileSync(process.argv[1], "utf8");
const m = src.match(/var WORDS = \("([^"]+)"\)\.split\(" "\);/);
if (!m) { console.error("cannot extract WORDS"); process.exit(1); }
const generated = Buffer.from(m[1].split(" ").join("\n") + "\n");
const vendored = fs.readFileSync(process.argv[2]);
if (!generated.equals(vendored)) process.exit(1);
' "$LITE/priv/static/crypto/crypto-app.js" wordlist/eff_short_wordlist_2_0.txt || { echo "DRIFT: wordlist differs" >&2; fail=1; }

sha256sum envelope/testdata/v1.json wordlist/eff_short_wordlist_2_0.txt
[ "$fail" = 0 ] && echo "no drift" || echo "DRIFT DETECTED" >&2
exit $fail
