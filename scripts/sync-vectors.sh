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

# Wordlist: regenerate from the web driver's array and compare.
node -e '
const fs = require("fs");
const src = fs.readFileSync(process.argv[1], "utf8");
const m = src.match(/var WORDS = \("([^"]+)"\)\.split\(" "\);/);
if (!m) { console.error("cannot extract WORDS"); process.exit(1); }
process.stdout.write(m[1].split(" ").join("\n") + "\n");
' "$LITE/priv/static/crypto/crypto-app.js" > /tmp/bp-wordlist.$$ || fail=1
cmp -s /tmp/bp-wordlist.$$ wordlist/eff_short_wordlist_2_0.txt || { echo "DRIFT: wordlist differs" >&2; fail=1; }
rm -f /tmp/bp-wordlist.$$

sha256sum envelope/testdata/v1.json wordlist/eff_short_wordlist_2_0.txt
[ "$fail" = 0 ] && echo "no drift" || echo "DRIFT DETECTED" >&2
exit $fail
