# Burnerpad CLI v1.0.0 — Current Server Alignment Plan

Status: implemented; local verification complete. The first scheduled Lite-main CI run and protected
release run necessarily remain post-merge acceptance steps.

This plan replaces the unreleased pre-1.0 client behavior with the current burnerpad-lite product contract.
It is the implementation checklist for the first public CLI release, `v1.0.0`. The accepted decisions are
recorded in ADR-0021 through ADR-0035. Superseded ADRs and the explicitly historical glossary remain only as
decision history.

There are no remaining product decisions in this plan. Discoveries that contradict the current
burnerpad-lite source are factual defects to resolve against the pinned server revision, not reasons to
restore an old CLI behavior.

## 0. Confirmed test seams

The approved design fixes four public seams for test-driven implementation:

- the `burnerpad` process boundary: argv, environment, stdin/TTY, stdout, stderr, files, and exit status;
- the burnerpad-lite HTTP boundary: exact request method/path/body and documented response semantics;
- the suite-`0x02` envelope/wordlist boundary: upstream vectors and canonical phrase behavior; and
- the official-client boundary: real browser ↔ Go CLI and Go CLI ↔ Go CLI flows against a real pinned server.

Tests assert behavior at these seams. Internal helpers may have focused property/fuzz tests, but orchestration
tests do not mock CLI-owned collaborators or freeze the current implementation structure.

## 1. Release boundary

The v1 CLI supports only:

- the current unversioned burnerpad-lite HTTP contract;
- suite `0x02` passphrase secrets created by the current official browser or CLI;
- non-empty create plaintext and revealed plaintext represented as well-formed UTF-8, with create input
  capped at 65,491 encoded bytes;
- current 26-character Crockford secret identifiers; and
- explicit create, reveal, revoke, and offline-recovery workflows.

The v1 CLI does not support suite `0x01`, fragment-bearing links, retired GET reveal, `/s/:id/report`,
historical 8-character IDs, HTTP 410/403 lifecycle semantics, automatic network retry, binary secrets,
free-form passphrases, command aliases, implicit reveal, or any pre-release pipe/JSON format.

## 2. Authoritative server contract

| Operation | Request | Successful response | Unavailable/rejected behavior |
|---|---|---|---|
| Create | `POST /api/secrets` with `{ "blob": "<canonical-base64url>", "ttl": <optional integer seconds> }` | `200 { "id", "mgmt_token", "ttl" }` | `400`/`413` definitive rejection; `429`/`503` temporary failure |
| Claim | `POST /api/secrets/:id/reveal` with `{}` and JSON content type | `200 { "blob" }` | generic `404` for every unavailable state |
| Revoke | `POST /api/secrets/:id/burn` with `{ "mgmt_token": "…" }` | `200 { "status": "burned" }` | generic `404` for bad token or unavailable secret |

Every response parser must require the fields needed to complete the operation, validate their types and
security-sensitive formats, reject trailing JSON values, cap the body, and ignore additive unknown object
fields. The CLI makes no `/api/stats` request or other runtime compatibility probe.

Create, claim, and revoke are mutations. Each user invocation sends at most one request for its operation.
No transport error, `Retry-After`, apparently early failure, or capacity response triggers an automatic
second request. A failure after the request may have reached the server, or after a success response becomes
incomplete or invalid, is reported as operation-specific outcome unknown.

## 3. Version-one command surface

There are no command aliases and no implicit command:

```text
burnerpad create
burnerpad reveal
burnerpad burn
burnerpad decrypt
burnerpad words
burnerpad completion
burnerpad version
burnerpad licenses
burnerpad help
```

`burn` is the only user-facing name for the revoke operation. `revoke` is not an alias. A share URL by itself
is an unknown command; claiming always requires `burnerpad reveal`.

### Global behavior

- `--json` selects the fixed machine result for create, reveal, burn, or decrypt. Utilities keep their
  purpose-specific text output and reject an inapplicable `--json`.
- Required security warnings, the resolved server, and information needed to recover a newly created secret
  are always emitted; there is no quiet mode.
- `--plain` keeps the accessible line-oriented prompt and terminal-safe plaintext renderer while avoiding
  raw mode, ANSI, and the alternate screen; `--no-color` and conventional `NO_COLOR` behavior remain.
- `--timeout DURATION` defaults to 12 seconds and must be positive.
- `--server ORIGIN` and `BURNERPAD_SERVER` configure create and bare-ID burn. The built-in default is
  `https://burnerpad.io`.
- There is no configuration file, persistent state, telemetry, crash reporter, update check,
  `--insecure-http`, or TLS-verification bypass.

### `create`

Accepted options:

- `--ttl DURATION`
- `--input FILE`
- `--ask`
- `--passphrase-file FILE`
- `--passphrase-fd FD`

The plaintext source is exactly one of the interactive composer, piped stdin, or `--input FILE`. Plaintext
never comes from argv or an environment variable. Supplying `--input` while stdin is already a pipe is an
ambiguous-source usage error. Validate non-empty UTF-8 and the 65,491-byte ceiling before encryption or
network access.

The CLI generates a seven-word phrase unless exactly one supplied-phrase source is selected. `--ask` uses
the controlling terminal. File and descriptor inputs are completely read and bounded before encryption.
There is no passphrase argv, environment, or stdin source and no `--words` option. Passphrase file input does
not accept `-`, and passphrase descriptors must be `3` or greater so neither form can silently consume a
primary standard stream.

An optional TTL must resolve to a positive whole number of seconds and fit the request integer type. Reject
zero, negative, fractional-second, and overflow values locally. Do not embed burnerpad.io's current TTL
ceiling. Display the returned effective `ttl`, and explicitly note when it differs from the requested value.

Interactive mode presents the share link and phrase as separate handoff artifacts and shows the management
token. A piped create using a generated phrase requires `--json`; otherwise the CLI would have no safe,
complete machine handoff. A piped create using a caller-supplied phrase may print only the link on stdout,
with the server and management token on stderr.

### `reveal`

Accepted options:

- `--ask`
- `--passphrase-file FILE`
- `--passphrase-fd FD`
- `--keep-blob FILE`
- `--out FILE`

Reveal accepts one full `http://` or `https://` `/s/<id>` share URL from argv, piped stdin, or a protected
terminal prompt. It does not accept a bare ID or path-only target. A URL in argv is accepted with a warning
that the destructive capability is now in shell history. A URL from stdin or the prompt does not warn.

The URL origin is authoritative. `--server` on reveal is accepted only so the CLI can warn that it is
ignored; `BURNERPAD_SERVER` is irrelevant and produces no warning. Human mode prints the URL's normalized
origin before the request. JSON success and error objects carry that origin in `server`.

Collect and validate the complete passphrase and every requested destination before claim. A valid but wrong
phrase may be corrected and retried locally against the held blob without another request. Invalid phrase
shape never causes a claim.

`--keep-blob FILE` reserves an exclusive mode-`0600` destination before claim, writes the canonical
unpadded-base64url blob plus one newline immediately after claim, and leaves it in place whether decryption
succeeds or fails. Without that option, abandoning a held blob produces a warning and never prints the blob.

### `burn`

Accepted options:

- `--server ORIGIN`
- `--token-file FILE`
- `--token-fd FD`

The invocation uses exactly one of these input forms:

- a piped create receipt containing `link`, `mgmt_token`, and `server`;
- a full share URL plus a token file, token fd, or protected token prompt; or
- a bare normalized ID plus a token file, token fd, or protected token prompt.

A receipt's `server` is authoritative and must match the origin in its `link`. A full URL's origin is
authoritative. A bare ID resolves `--server`, then `BURNERPAD_SERVER`, then the default. An explicitly
supplied but irrelevant server option warns. Mixed receipt/argument or token sources are rejected. There is
no `--token VALUE`, token environment variable, or extra confirmation prompt. Token file input does not
accept `-`, and token descriptors must be `3` or greater.

### `decrypt`

Accepted options:

- `--blob-file FILE`, with `-` selecting stdin
- `--ask`
- `--passphrase-file FILE`
- `--passphrase-fd FD`
- `--out FILE`

Decrypt is structurally local-only and never constructs a network client. Its blob input is canonical
unpadded base64url text with one optional terminal newline; raw envelope bytes and JSON wrappers are rejected.
It applies the same suite-`0x02`, passphrase, UTF-8, output, and JSON rules as reveal.

## 4. Canonical passphrase behavior

- Generated phrases contain exactly seven distinct uniformly sampled words from the shared EFF wordlist.
- Supplied, pasted, and typed phrases contain 7–64 distinct members of that list.
- Input parsing accepts ASCII whitespace around and between words, then derives the cryptographic phrase by
  lowercasing the accepted words and joining them with one ASCII space.
- It never changes a word or its order. Off-list words, duplicates, excessive token length, excessive total
  input, and out-of-range word counts fail without echoing a token.
- Create, reveal, and decrypt use the same parser. There is no free-form escape.
- Raw-mode list autocomplete retains explicit Space, Tab, or Enter commitment, atomic paste, and the
  unique-prefix ghost text. Plain mode applies the identical parser without terminal control sequences.
- Only generated phrases carry the documented entropy claim; supplied phrases are described as compatible,
  not guaranteed strong.

## 5. Output and machine contract

Human reveal and decrypt use one terminal-safe renderer whenever stdout is a terminal, with an alternate
screen when available and the same escaped rendition in plain/fallback mode. Graphic UTF-8 remains readable;
LF and CRLF become renderer-owned line breaks; other control, format, and non-graphic characters become
inert visible escapes. The rendition is not a byte-forensic export. Non-terminal stdout and `--out FILE`
receive exact authenticated UTF-8 bytes; `--out` reserves an exclusive mode-`0600` destination and never
overwrites an existing path. `--force` is removed. Destination setup happens before a network claim. If a
reserved output file is no longer needed because claim failed, remove only the file created by that
invocation.

Reveal and decrypt select exactly one explicit plaintext destination: `--json` or `--out`. With neither,
terminal stdout selects the safe viewer and non-terminal stdout receives exact plaintext. JSON is byte-exact
after decoding its `plaintext` string. There is no built-in clipboard destination because a terminal clipboard
request cannot prove acceptance or completeness, while helper executables add a PATH and platform trust
boundary. Callers may pipe exact stdout to a tool they select; destructive reveal should use `--keep-blob`
when that external handoff may need recovery.

Successful JSON lines are exactly:

```json
{"status":"created","server":"https://burnerpad.io","link":"…","phrase":"…","mgmt_token":"…","ttl":86400}
{"status":"revealed","server":"https://burnerpad.io","plaintext":"…"}
{"status":"burned","server":"https://burnerpad.io"}
{"status":"decrypted","plaintext":"…"}
```

Errors use:

```json
{"status":"error","code":"claim_outcome_unknown","message":"…","server":"https://burnerpad.io"}
```

`retry_after` is the only optional addition and appears only when the server supplied it. Error JSON never
contains a URL, ID, phrase, management token, ciphertext, plaintext, raw response, filesystem path, or nested
error. Use a closed code vocabulary grouped under the frozen exit meanings:

| Exit | Codes |
|---:|---|
| `2` | `invalid_command`, `invalid_option`, `invalid_input`, `invalid_credential_source` |
| `3` | `local_io_failed` |
| `4` | `secret_unavailable` |
| `5` | `passphrase_failed`, `plaintext_invalid` |
| `6` | `server_rejected` |
| `7` | `network_unavailable`, `rate_limited`, `service_unavailable` |
| `8` | `invalid_server_response`, `unsupported_secret` |
| `9` | `create_outcome_unknown`, `claim_outcome_unknown`, `revoke_outcome_unknown` |
| `10` | `internal` |

The first SIGINT or SIGTERM cancels a Run-owned context and joins dispatch so terminal, recovery, and output
cleanup attempts can finish. If cancellation leaves an already-transmitted mutation unconfirmed,
its operation-specific outcome-unknown error and exit `9` take precedence. Definitive command/local failures
found while joining also remain authoritative; a confirmed success completes its required handoff and then
returns `130`/`143`. Signal exits do not manufacture an ordinary JSON error. A second production signal
restores immediate OS termination. Confirmed one-time plaintext that becomes ready after cancellation uses
persistent plain terminal output rather than being flashed and erased by a canceled wait. Exit zero means the
requested artifact reached its selected destination.

## 6. Implementation work

### A. Replace the product-facing crypto surface

- In `envelope/`, remove suite-`0x01` encryption, decryption, key, and fragment APIs from the Go product.
  Retain suite-`0x02` AES-256-GCM, PBKDF2-HMAC-SHA256, canonical-base64url, randomness seams, memory wiping,
  and applicable upstream known-answer/negative tests.
- Make the supported vector loader hash the raw pinned file, select and require every suite-`0x02` case, and
  execute both generic encoding classes rather than claiming whole-spec support. A newly added applicable
  vector or expectation must fail CI until handled.
- Keep the upstream language-neutral specification as provenance even if it documents suites outside this
  product; clearly state that the CLI conformance claim is the supported subset.
- Remove fragment-shaped error categories, scrubber exceptions, differential-driver modes, and fuzz cases
  that exist only for suite `0x01`.

Primary files: `envelope/*.go`, `envelope/*_test.go`, `internal/difftest/`, `spec/SPEC.md`,
`scripts/sync-vectors.sh`, and `scripts/check-deps.sh`.

### B. Replace identifier and target parsing

- Make `internal/id.Normalize` produce exactly 26 canonical uppercase Crockford characters after applying
  lowercase, hyphen, and `I`/`L`/`O` aliases. Reject every other length or character.
- Split parsing by operation: reveal parses only a full share URL; burn parses a full share URL or bare ID.
  Reject fragments, credentials, query strings, path aliases, escaped IDs, and trailing path segments before
  network access.
- Return the normalized origin and ID as separate values so request construction never reuses an untrusted
  URL string.

Primary files: `internal/id/id.go`, `internal/id/parse.go`, and their unit/fuzz tests.

### C. Rewrite the HTTP client around current POST mutations

- Replace `Take`/burning GET with `Reveal` using `POST /api/secrets/:id/reveal` and `{}`.
- Move burn from `/s/:id/burn` to `/api/secrets/:id/burn`; remove `Report` entirely.
- Extend `CreateResult` with the required effective TTL. Validate the returned 26-character ID and canonical
  32-byte management token before reporting success.
- Replace `DisallowUnknownFields` with required-field validation plus additive-field tolerance and a trailing
  data check. Keep bounded response reads and fixed diagnostics that never quote response bytes.
- Remove all retry loops, sleeps, jitter, rate-limit countdowns, and HTTP/1.1/keep-alive workarounds.
- Use normal Go HTTP protocol negotiation, proxy environment handling, platform roots, no redirects, HTTPS
  remotely, and HTTP only for loopback. Default to one 12-second total deadline.
- Track whether the request may have reached the transport. Classify definite pre-send inability as temporary
  unavailability and every incomplete/invalid post-send mutation result as the corresponding outcome unknown.
  A documented error status is definitive only for the operation that lists it; every other final status is
  outcome unknown after transmission. Reserve exit 8 for a complete response that establishes its outcome
  but violates the expected representation, or for a fully received secret using an unsupported envelope.

Primary files: `internal/api/api.go`, `internal/api/client.go`, `internal/api/errors.go`,
`internal/api/windows.go`, and tests. Delete `internal/api/retry.go` and obsolete exactly-once tests.

### D. Rebuild dispatch, flags, and configuration

- Replace alias/implicit dispatch with an exact command switch. Remove `report` from `subFlags` and dispatch.
- Register only the agreed flags, detect duplicate/mixed sources, and remove suite flags, `--words`,
  `--insecure-http`, `--force`, fragment flags, `--token`, and obsolete command shorthands.
- Rename secret file input to `--input` and add `--token-fd`. Retire built-in clipboard delivery because its
  terminal protocol cannot establish destination success.
- Resolve server origin per operation instead of globally redirecting every target. Always emit the human
  server line before network access; put it in every network JSON result.
- Remove `BURNERPAD_PASSPHRASE` and every obsolete environment lookup. Set the timeout default to 12 seconds.

Primary files: `internal/cli/run.go`, `internal/cli/flags.go`, `internal/cli/config.go`,
`internal/cli/env.go`, `cmd/burnerpad/main.go`, and flag/dispatch tests.

### E. Rebuild create

- Replace suite selection with unconditional suite `0x02`.
- Implement exclusive composer/stdin/`--input` selection and shared UTF-8/size validation.
- Replace adjustable phrase generation and free-form input with the canonical phrase parser.
- Parse whole-second TTLs without a local deployment ceiling; consume and show the server's effective TTL.
- Implement the three agreed handoff modes and the exact created JSON receipt.
- Treat any uncertain create response as `create_outcome_unknown` and never retry it.

Primary files: `internal/cli/create.go`, `internal/cli/passphrase.go`, `wordlist/phrase.go`,
`wordlist/validate.go`, and their tests.

### F. Rebuild reveal and recovery

- Require a full URL and collect the canonical passphrase before claim.
- Pre-open requested output/recovery destinations and preflight the terminal viewer before the POST.
- Persist an opted-in recovery blob immediately after claim, then perform unlimited local phrase correction
  without another network request.
- Validate decrypted bytes as UTF-8 and deliver them to exactly one viewer/stdout/file/JSON
  destination. Route both terminal modes through the safe renderer; retain original bytes for every other
  destination. Never include the held blob in an error object or diagnostic.
- Simplify offline decrypt to suite `0x02`, one base64url blob format, the shared phrase collector, and the same
  plaintext destinations.

Primary files: `internal/cli/reveal.go`, `internal/cli/openflow.go`, `internal/cli/decrypt.go`,
`internal/cli/output.go`, `internal/secret/`, and tests.

### G. Rebuild burn

- Parse the new create receipt (`link`, `mgmt_token`, `server`) additively and validate its internal origin
  agreement without echoing sensitive values.
- Implement mutually exclusive receipt/file/fd/prompt token acquisition and canonical token validation.
- Call the current endpoint once, map the common `404` to unavailable, and report uncertain completion as
  `revoke_outcome_unknown`.
- Delete `internal/cli/report.go` and all report tests/help/completion entries.

Primary files: `internal/cli/burn.go`, `internal/cli/flags.go`, `internal/cli/output.go`, and tests.

### H. Rebuild output and errors

- Replace every JSON struct with the four success shapes and one flat error shape above.
- Replace the old 0–11 exit table with the accepted 0, 2–10, 130, and 143 meanings. Local I/O becomes exit 3;
  internal failure becomes exit 10.
- Remove IDs, suite labels, base64 plaintext, output paths, response bodies, and recovery blobs from machine
  output. Ensure decoding JSON's UTF-8 plaintext string reproduces the authenticated bytes exactly.
- Golden-test every code, field order if promised by documentation, secret-redaction rule, human server line,
  history warning, and unknown-outcome message.

Primary files: `internal/cli/output.go`, `internal/cli/exit.go`, `internal/cli/help.go`, and golden tests.

### I. Retain and simplify terminal behavior

- Keep the pure autocomplete state machine, terminal restoration, bracketed-paste hygiene, plain-mode
  accessibility, shared safe plaintext renderer, alternate-screen viewer, and best-effort memory wiping. Test
  both viewer modes against ESC/OSC/CSI, C0/C1, DEL, bidi/format characters, malformed UTF-8, and LF/CRLF
  without allowing untrusted terminal controls through.
- Remove free-form phrase entry, binary detection/output branches, generated-word editing beyond reroll, and
  gestures for adjustable word counts.
- Keep clipboard protocols and helper executables outside the destination boundary.

Primary files: `internal/term/`, with PTY coverage for prompt, retry, viewer, and signal cleanup.

### J. Replace documentation and the completion surface

- Rewrite `README.md`, `docs/ARCHITECTURE.md`, `docs/TASKS.md`, and `docs/burnerpad.1.scd` from this plan rather
  than editing retired examples piecemeal.
- Keep `CONTEXT.md` as the canonical glossary and `docs/GLOSSARY.md` explicitly historical.
- Serve Bash, Zsh, Fish, and PowerShell completions from one in-binary definition matching the exact no-alias
  command/flag set; do not keep duplicate checked-in artifacts.
- Update `SECURITY.md`, `CONTRIBUTING.md`, release examples, and comments that claim GET/410/403, fragments,
  binary support, retries, output blob recovery, or pre-v1 release channels.

### K. Add real-server and browser interoperability gates

- Add one repository file containing the reviewed burnerpad-lite commit SHA. Every PR and release checks out
  that exact revision and builds its real application/container; do not use a hand-written API mock as the
  compatibility authority.
- Add a scheduled workflow that checks out burnerpad-lite `main` and runs the same compatibility suite. Its
  failure is an early drift alert, not permission to weaken the pinned release gate.
- Exercise the actual official clients in all three directions against one real server:
  1. browser creates, Go CLI reveals;
  2. Go CLI creates, browser reveals; and
  3. Go CLI creates, a second Go CLI process reveals.
- Exercise create receipt burn, full-URL burn, bare-ID custom-server burn, wrong token/unavailable collapse,
  second claim, expiry, effective TTL/default/clamp, lowercase/alias ID input, and additive response fields.
- Inject lost connections, truncated/invalid success bodies, redirects, rate limits, and delayed responses.
  Assert one request per invocation, correct outcome-unknown classification, no secret material in logs, and
  no egress beyond the selected origin.
- Keep unit mocks only for deterministic failure injection. Rename or replace `internal/cli/clitest/mockserver.go`
  so it models the pinned current contract and cannot be mistaken for the integration authority.

Primary files: `.github/workflows/ci.yml`, `.github/workflows/publish-release.yml`, new pinned and scheduled integration
workflows, the Lite revision file, `internal/cli/clitest/`, and browser orchestration scripts.

### L. Prepare the tag-derived v1 release

- Change the changelog's unreleased entry to describe only the aligned product and prepare `v1.0.0`.
- Keep the binary version derived from the Git tag through linker flags; do not add a source-code version
  constant that requires a release commit.
- Make the release workflow run the pinned real-server/browser gate before publishing.
- Render the version/checksum-pinned `install.sh` as a `v1.0.0` release artifact from the tag and generated
  checksums. Do not require a post-release source commit to change its version.
- Remove `v0.1.0`/`v0.2.x` milestones and stale channel comments. Reconcile the first-release distribution
  list with ADR-0019 and verify every advertised channel exists before listing it as official.
- Re-run cross-platform tests, `go vet`, formatting, supported suite vectors, fuzz smoke, `govulncheck`, size,
  SBOM, signatures, provenance, and reproducibility checks on the tag commit.
- Build the tag with the latest supported stable Go patch and keep the `go 1.25` module compatibility floor
  unless the dependency graph makes a reviewed increase necessary.

## 7. Required acceptance evidence

The implementation is ready to tag only when all of the following are true:

- `go test ./...` passes on Linux, macOS, and Windows.
- No shipped command, flag, help entry, completion, source import path, or test advertises retired behavior.
- All applicable suite-`0x02` vectors and browser/CLI differential checks pass.
- The pinned real Lite revision passes all three interoperability directions.
- The scheduled Lite-main job exists and has produced a successful run.
- Create, claim, and revoke each make exactly one request under every tested failure window.
- Every network operation exposes the actual server; reveal cannot be redirected by configuration.
- Every JSON shape and exit status matches ADR-0028, including unknown outcomes and `retry_after`.
- Secret-bearing argv/environment inputs are structurally absent, and redaction tests cover every diagnostic.
- Recovery and plaintext files are exclusive and mode `0600`; existing files are never overwritten.
- Both terminal modes inertly escape untrusted controls while byte-exact destinations round-trip the
  authenticated plaintext.
- The README, man page, help, completions, changelog, architecture, release guide, and executable agree.
- The `v1.0.0` tag alone supplies the released version, and the release gate produces verified artifacts
  without a follow-up version-bump commit.
