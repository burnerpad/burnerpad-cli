# Vendored Wycheproof test vectors

Adversarial vector sets from the Wycheproof project, exercised by
`envelope/wycheproof_test.go` (white-box, same-package).

## Provenance

- **Source repository:** https://github.com/C2SP/wycheproof
  (the maintained home of the Wycheproof project; the legacy
  `google/wycheproof` `testvectors/` layout no longer exists upstream —
  `testvectors_v1/` is the only layout at the pinned commit)
- **Commit:** `4bb5ed764261bb3699f93567998a3467d3cc9785`
  (branch `main`, committed 2026-08-17T17:14:50Z; the repository publishes no
  release tags)
- **Retrieved:** 2026-08-18, via
  `https://raw.githubusercontent.com/C2SP/wycheproof/4bb5ed764261bb3699f93567998a3467d3cc9785/testvectors_v1/<file>`
- **License:** Apache-2.0 (see `LICENSE` in the source repository)

## Files

Both files are byte-identical to upstream at the pinned commit — no
provenance header is injected into the JSON itself, so anyone can re-verify
them against the raw URLs above. This README and the sha256 pins in
`wycheproof_test.go` carry the provenance instead.

| File | sha256 |
|---|---|
| `aes_gcm_test.json` | `985e5ecc172e181eaf49e89508b9470dcf478002eb7e8559c707eb42dc97dfe7` |
| `pbkdf2_hmacsha256_test.json` | `1bf37af2cefe40c829ee9ecebb3505bb6424be8824bc97aa3e1c2076e860d192` |

The same sha256 values are pinned as constants in
`envelope/wycheproof_test.go`; the files can only change together with a
reviewed re-pin (same discipline as `testdata/v1.json`, §14.1).

## What the tests select

- **AES-GCM** (`aes_gcm_test.json`, 316 cases total): only the exact
  envelope-v1 profile — keySize 256 ∧ ivSize 96 ∧ tagSize 128 — is driven
  through the unexported `open()`; at the pinned commit that is 66 cases
  (39 valid, 27 invalid, 0 acceptable), counts pinned in the test.
  Out-of-profile groups (other key/IV sizes) exercise GCM shapes this
  package can never produce or accept and are deliberately not run.
- **PBKDF2-HMAC-SHA256** (`pbkdf2_hmacsha256_test.json`, 60 cases, all
  "valid" at the pinned commit): every case is pinned against stdlib
  `crypto/pbkdf2` at its exact dkLen, and additionally driven through the
  production `deriveKey` path via the `kdfIter` test seam using the
  RFC 8018 §5.2 prefix property.

At vendoring time every PBKDF2 case was also cross-checked against an
independent implementation (Python 3 `hashlib.pbkdf2_hmac`): 60/60 match.
