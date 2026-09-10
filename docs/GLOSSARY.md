# Burnerpad CLI — Historical Design Glossary

> This document records the unreleased pre-1.0 design and is being reconciled with the current
> burnerpad-lite contract. The canonical domain language now lives in [`../CONTEXT.md`](../CONTEXT.md).
> Entries that refer to a burning GET, exactly-once delivery, HTTP 410, retired endpoints, or the former
> suite-by-TTY policy are historical rather than normative.

> Section references (§) point into ARCHITECTURE.md unless marked *(SPEC)* — the envelope spec at
> `burnerpad-lite/priv/static/vendor/crypto-js/SPEC.md` — or *(server)* — `burnerpad-lite`.


---

## 1. Domain model

```
                                ┌─ suite 0x01: fragment (the key, base64url) travels IN the URL
  plaintext ──encrypt──▶ envelope (blob)
                                └─ suite 0x02: passphrase travels OUT OF BAND; blob carries salt

  blob ──POST /api/secrets──▶ server row {id, blob, sha256(mgmt_token), expires_at}   (RAM only)

  server row ──the one burning GET /api/secrets/:id──▶ blob in CLI RAM ──decrypt──▶ plaintext
             └── burn (mgmt token) / TTL expiry / restart ──▶ gone
```

**Secret lifecycle states** (server-side): *minted* → *stored (live)* → exactly one of
*taken* (burned by the one read) | *burned* (early revoke via mgmt token) | *expired* (TTL sweep) |
*purged* (operator takedown) | *lost* (server restart). All terminal states collapse into one
observable **gone** state (HTTP 410 on the take endpoints) — deliberately indistinguishable.

**CLI-side reveal phases** (§7.1): *pre-burn* (parse, normalize, collect credential — every failure
is free) → **the burning GET** (the single irreversible act) → *post-burn* (decrypt, retry locally,
preserve the blob — the process now holds the only copy in the universe).

---

## 2. Cryptography & wire format

| Term | Meaning |
|---|---|
| **envelope** / **blob** | The single opaque ciphertext byte string a client mints and the server stores verbatim: `suite ‖ header ‖ ct‖tag`. The server never parses it *(SPEC §1)*. "Blob" is the same bytes viewed as a storage/transport unit (base64url in JSON). |
| **suite** | The first envelope byte; selects the entire format, globally and forever. `0x01` = `SYMMETRIC_AESGCM_V1` (link mode), `0x02` = `PASSPHRASE_PBKDF2_AESGCM_V1` (passphrase mode). Unknown suite ⇒ `reject_unsupported_suite`, never guess *(SPEC §2)*. |
| **link mode** (0x01) | The 32-byte random key is carried only in the URL `#fragment`; the URL alone is the full capability. Blob = `0x01 ‖ iv(12) ‖ ct‖tag`; min 29 bytes; AAD = `0x01 0x01`. |
| **passphrase mode** (0x02) | Key = PBKDF2-HMAC-SHA256(passphrase, salt, 600 000, 32). Blob = `0x02 ‖ salt(16) ‖ iv(12) ‖ ct‖tag`; min 45 bytes; AAD = `0x02 0x02 ‖ salt ‖ iv` (whole header authenticated). No fragment. |
| **fragment** | Everything after `#` in a share URL — for 0x01 it **is** the key (canonical base64url of 32 bytes, 43 chars). Never sent by browsers, never sent/logged by the CLI (§17). |
| **canonical base64url** | Unpadded URL-safe base64 admitting exactly one encoding per byte string: alphabet `[A-Za-z0-9_-]` only, no `=`, no whitespace, `len%4 ≠ 1`, zero trailing bits. Enforced by the single decode gate `DecodeCanonical` (§11.4, M1). |
| **AAD** | Additional Authenticated Data bound into the GCM tag: `suite ‖ spec_version (‖ salt ‖ iv)`. Defeats suite-confusion/downgrade: reinterpreting a blob under another suite is an `auth_fail` *(SPEC §5)*. |
| **canonical passphrase form** | Lowercase words joined by single spaces, raw UTF-8, **no Unicode normalization, no trimming beyond the prompt's own construction**. The exact bytes fed to PBKDF2. The CLI prompt makes any other form unrepresentable (§7); caller-supplied phrases are used byte-verbatim (§7.5). |
| **canonical reject reasons** | The closed five-error surface of the crypto layer: `reject_unsupported_suite`, `reject_truncated`, `reject_bad_key`, `reject_bad_encoding`, `auth_fail` *(SPEC §6)*. Vector vocabulary — never reworded, never extended. |
| **KAT** | Known-Answer Test. Decrypt KATs anchor cross-implementation compatibility; encrypt KATs replay fixed randomness through the unexported seam (§14.1). |
| **vectors** | `vectors/v1.json` — the language-neutral conformance set (7 decrypt KATs, 6 encrypt KATs, 25 negatives, 3+6 encoding cases). Conformance = green on the entire set; vendored verbatim and sha256-pinned (§14.1). |
| **single-fault vector** | A negative vector triggering exactly one reject reason. Implementations must agree on reasons for single-fault inputs; on multi-fault garbage only verdicts must agree (v1 pins no precedence) *(SPEC §6)*. |
| **randomness seam** | The unexported `randRead` variable swapped only by same-package tests — the only way fixed randomness enters; no fixed-IV public entrypoint exists, making (key, nonce) reuse structurally impossible (§11.7). |

## 3. Identifiers, tokens, URLs

| Term | Meaning |
|---|---|
| **id** | The short public identifier (`B3ZRJ8P0`): Crockford base32, server default 8 chars ≈ 40 bits. Not a confidentiality control — guessing one enables only burn-griefing, bounded by rate limits *(server §8)*. |
| **Crockford normalization** | Uppercase → strip `-` → fold `I`,`L`→`1`, `O`→`0` → validate alphabet `0123456789ABCDEFGHJKMNPQRSTVWXYZ`, non-empty, ≤ 64 bytes. The CLI mirrors the server's `Store.normalize/1` verbatim and normalizes **before** the request (§17). |
| **share URL** | `https://<host>/s/<id>[#fragment]`. The URL's origin selects the server (precedence over `--server`, §4.3). |
| **mgmt token** | 32 random bytes, base64url (43 chars), returned once by create. A **destroy-only capability**: it can burn, never read — which is why it (alone) may appear in argv (§4.4). The server stores only its SHA-256. |
| **receipt** | The `--json` create output persisted by the user (`> receipt.json`). Parsed **leniently** by `burn` (only `id`, `mgmt_token` required; `url` never read) (§15.1). |

## 4. Server interaction

| Term | Meaning |
|---|---|
| **the burning GET** / **take** | `GET /api/secrets/:id` — the server's `:ets.take` destroys the row *before* the first response byte. Issued at most once per invocation, never auto-retried after it may have reached the server (§7.1, §16). |
| **burn-on-read** / **exactly-once** | The first successful take destroys the secret atomically; no second read exists for anyone, by design. |
| **burn** | Early revoke: `POST /s/:id/burn` with the mgmt token. Destroys without reading. 403 = invalid token *or* already gone (indistinguishable, by server design). |
| **gone** | The single observable terminal state: HTTP 410 on take. Already-read, expired, and never-existed are deliberately identical (no existence oracle). |
| **failure windows W0–W8** | The exhaustive classification of take-path failures (§16.1): W0 provably-unsent (retryable) · W1 ambiguous request-written (never retry, exit 9 `take_ambiguous`) · W2 burned-response-lost (exit 9 `take_lost`) · W3 burned-parse-failed (exit 8) · W4 burned-decrypt-failed (local retry / blob preservation) · W5 definitive 410 (exit 4) · W6 definitive 429/503, origin- or edge-generated — both provably burn-free, retry-safe · W7 gateway 5xx (502/504/520/521/524/530) after the request may have reached the app — ambiguous, exit 9, never retried · W8 edge-to-origin connect failures (522/523/525/526) — provably unsent, exit 3. |
| **blob preservation** | The post-burn invariant: the process never exits — error, wrong phrase, Ctrl+C — without preserving the fetched ciphertext (stderr print, `--keep-blob`, or JSON `blob` field). `burnerpad decrypt` is the offline recovery half (§7.4). |
| **local retry** | Wrong-passphrase attempts after the burn are free, unlimited, and purely local against the in-RAM blob — mirroring the web client's held-blob retry. |
| **store-full 503** vs **abuse 503** | Two origins: `MAX_SECRETS` reached on create (no `Retry-After`; jittered backoff) vs the global-ceiling abuse shed (has `Retry-After`; honored). Both exit 7 (§16.2, §16.4). |

## 5. CLI surface & interaction

| Term | Meaning |
|---|---|
| **artifact** | The one thing stdout carries: the share URL (create), a terminal-safe plaintext rendition on TTY stdout or exact plaintext on piped stdout (reveal/decrypt), or the `--json` object. Everything else is stderr/tty (§5.1). |
| **implicit reveal** | A first argument that is not a subcommand but parses as URL/id is treated as `reveal <arg>` (§4.2). |
| **suite-by-audience rule** | Interactive stdout (TTY) ⇒ mint 0x02; piped stdout ⇒ mint 0x01. Always announced on stderr; overridden by `-P`/`-L` (§5.2). |
| **two-channel handoff** | The 0x02 create ending: link and phrase presented as two blocks meant to travel by two different channels (§6). |
| **list-locked autocomplete** | The showpiece reveal prompt: typing constrained to the embedded wordlist; a keystroke that would leave zero candidates is rejected (bell), so a malformed phrase is unrepresentable (§7.2). |
| **ghost text** | The dim completion rendered after the cursor once the prefix is unique (SGR 2). Display only — commitment is always an explicit Space/Enter on a fully visible word ("no auto-commit", §7.2). |
| **commit** | Accepting the current word into the phrase (Space/Enter when unambiguous); committed words render in full and count toward the ≥ 7 gate. |
| **free-form mode** | Ctrl+O fallback: masked line entry for non-wordlist passphrases other clients may mint (list-locking is a burnerpad-phrase optimization, not a format rule) (§7.2). |
| **plain mode** | `--plain` / `BURNERPAD_PLAIN=1` / `CI` / `TERM=dumb`: line-based prompts, no raw mode, no ANSI; the accessibility-first equivalent path (§7.6). |
| **alternate-screen viewer** | The `less`-style reveal display on a TTY: a terminal-safe plaintext rendition is shown on the alternate screen; `q` returns while the alternate screen limits primary-scrollback exposure (§8.1). |
| **OSC 52** | A retired pre-release terminal clipboard mechanism. The CLI removed it because the terminal cannot acknowledge acceptance or payload completeness; see ADR-0035. |
| **bracketed paste** | Terminal mode 2004: a paste arrives delimited by markers, letting the prompt validate a whole phrase atomically — all words commit, or the entire paste is rejected (§7.2). |
| **stream discipline** | The §5.1 contract: stdout = artifact only; stderr = everything else; interactivity decided solely by `term.IsTerminal` per fd. |
| **scrubber** | `output.scrub()` at the single output boundary: redacts `#…` patterns from every error/log line — defense in depth for the bug not yet written (§17, M5). |

## 6. Wordlist & phrases

| Term | Meaning |
|---|---|
| **the wordlist** | EFF Short Wordlist #2, 1296 (= 6⁴) words, CC BY 3.0, embedded; byte-identical to the web client's array. Frozen: any edit breaks phrase-entry UX and fails CI (§13). |
| **unique-3-prefix property** | Every word is uniquely determined by its first 3 characters — the load-bearing property behind 3-keystroke word entry. Tested, not assumed. |
| **edit-distance ≥ 3** | Minimum pairwise plain-Levenshtein distance of the list (exactly 3) — the spoken-channel property: a misheard word is never one edit from another list word. (Five pairs sit at Damerau distance 2; the CI asserts the Levenshtein bound EFF guarantees.) |
| **generated phrase** | `Phrase(n)`: n ≥ 7 **distinct** words drawn by rejection-sampled uniform indices (~10.34 bits/word; 7 words ≈ 72.4 bits), joined canonically. The CLI, unlike the web, has no path to fewer than 7 generated words (§6). |
| **word count flag** | `-w/--words N` (7–16): raises the generated count; implies `-P`. |

## 7. Engineering & release terms

| Term | Meaning |
|---|---|
| **dependency freeze** | `go.mod` lists exactly `golang.org/x/term` (+ transitive `x/sys`); CI fails on growth (M6, §19). |
| **package boundary rule** | `envelope` and `wordlist` are public, stdlib-only, extractable; `internal/cli` is the only package that imports everything (§19). |
| **conformance gate** | The entire vector set green as a merge *and* release condition; a red vector can neither merge nor ship (M2, §14.1). |
| **differential testing** | Randomized Go ↔ JS-reference cross-checks (encrypt one side, decrypt the other; verdict/reason agreement on mutations) (§14.2). |
| **spec-drift job** | CI job byte-comparing vendored SPEC/vectors/wordlist against the pinned upstream tag (§14.1). |
| **reproducible build** | Same toolchain + flags + module inputs ⇒ bit-identical binaries; independently re-verified post-release by `repro-verify` (§22.3). |
| **keyless signing** | Sigstore cosign via the release workflow's OIDC identity — no long-lived private key exists to steal; every signature lands in the Rekor transparency log (§23). |
| **size gate** | CI hard-fails any target binary > 9 MiB — an import-graph drift detector, paired with a `go list -deps` allowlist (M10, §22.2). |
| **golden test** | A test asserting fixed user-facing text byte-for-byte (the exit-9 messages, `--json` shapes); the wording is part of the compatibility promise (§16.3, §20). |
| **secret hygiene** | The §12 posture: secrets are heap `[]byte` (never `string`), wiped after last use, core dumps/ptrace disabled at startup, mlock best-effort — documented honestly as *narrowing windows*, not erasure. |
