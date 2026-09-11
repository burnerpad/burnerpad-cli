# Burnerpad CLI architecture

This document specifies the version-one architecture. Domain terminology lives in [`CONTEXT.md`](../CONTEXT.md),
and the decisions behind the current boundary are listed in the [ADR index](adr/README.md).

## Product boundary

The CLI targets only the current burnerpad-lite product contract:

- passphrase suite `0x02`;
- current 26-character Crockford IDs;
- the three mutation endpoints under `/api/secrets`;
- UTF-8 text secrets; and
- cross-client compatibility with the current official browser.

There is no compatibility mode for fragment links, suite `0x01`, short IDs, destructive GET, report,
binary output, free-form phrases, implicit reveal, aliases, retries, configuration files, or pre-v1
machine output. The language-neutral envelope specification remains vendored as provenance; the product
claim is explicitly its suite-`0x02` subset.

## Components

```text
cmd/burnerpad
  └─ internal/cli       process grammar, protected inputs, output and exit contract
       ├─ internal/id   operation-specific target parsing and ID normalization
       ├─ internal/api  one-attempt current burnerpad-lite HTTP client
       ├─ internal/term controlling-terminal prompts and viewer
       ├─ internal/secret owner-only files and best-effort memory hygiene
       ├─ wordlist      shared EFF list, generation and canonicalization
       └─ envelope      suite-0x02 encryption, decryption and canonical base64url
```

The process boundary is injected through `cli.Env`, making argv, environment, standard streams, TTY
availability, build identity, and signals explicit test inputs. Network code receives a separately parsed
origin and canonical ID; it never receives an untrusted full share URL.

## Public commands

The exact command set is `create`, `reveal`, `burn`, `decrypt`, `words`, `completion`, `version`, `licenses`,
and `help`. Dispatch recognizes only that ordered command schema. There are no aliases and a URL without
`reveal` is an invalid command. `help COMMAND`, `COMMAND --help`, and `COMMAND -h` render the same
command-specific help before parsing or execution. Root `--help`/`-h` print general help, and root
`--version` prints version identity.

Network operations accept `--timeout`; create and burn accept `--server`, while reveal accepts it only to
warn that its URL is authoritative. All four operations accept `--json`. The terminal presentation controls
`--plain` and `--no-color` apply only to create, reveal, and decrypt; burn has no colored or structured
terminal UI for them to change. `--plain` changes terminal mechanics, not plaintext safety: it uses the same
escaped viewer rendition without raw mode, ANSI, or the alternate screen. The default deadline is 12 seconds.
`BURNERPAD_SERVER` applies only to create and bare-ID burn. The built-in server is `https://burnerpad.io`.

## Server selection

Server selection is operation-specific:

| Operation/input | Authoritative origin |
|---|---|
| Create | `--server` → `BURNERPAD_SERVER` → burnerpad.io |
| Reveal | Full share URL origin; configuration cannot redirect it |
| Burn receipt | Receipt server, which must equal its link origin |
| Burn full URL | URL origin |
| Burn bare ID | `--server` → `BURNERPAD_SERVER` → burnerpad.io |
| Decrypt | No origin and no HTTP client |

Every network command writes `burnerpad: server: <origin>` to stderr before its request. Every network JSON
success or error includes the origin. An irrelevant explicit `--server` produces a
warning; an irrelevant environment value is silently ignored.

## HTTP contract

| Operation | Request | Success | Definitive failure |
|---|---|---|---|
| Create | `POST /api/secrets`, `{blob, ttl?}` | `200 {id, mgmt_token, ttl}` | `400`/`413`, `429`, `503` |
| Claim | `POST /api/secrets/:id/reveal`, `{}` | `200 {blob}` | generic `404`, `429`, `503` |
| Revoke | `POST /api/secrets/:id/burn`, `{mgmt_token}` | `200 {status:"burned"}` | generic `404`, `429`, `503` |

Only a status listed for that operation is a definitive failure. Once a request may have been transmitted,
every other final status—including another `2xx`, a refused redirect, an operation-inapplicable `4xx`, or an
unexpected `5xx`—is operation-specific outcome unknown. A `200` with an incomplete or invalid success body
is outcome unknown as well.

All request bodies are JSON and blobs use canonical unpadded base64url. Response bodies are capped at
200,000 bytes. Parsers require the fields needed by the operation, validate IDs/tokens/blob encoding and
types, reject trailing JSON values, and tolerate additive object fields.

Create, claim, and revoke are mutations. One invocation sends exactly one operation request. There is no
transport retry, status retry, backoff, or `Retry-After` waiting. `httptrace.WroteHeaders` separates a
definite pre-send temporary failure from a request that may have reached the server. Any incomplete or
invalid success after the mutation might have happened becomes operation-specific outcome unknown, exit 9.

The client clones Go's default transport to retain proxy-environment and normal HTTP/1.1/HTTP/2 behavior,
uses platform trust roots with TLS 1.2 minimum, permits plaintext HTTP only for loopback, and refuses all
redirects. No runtime stats or compatibility probe is made.

## Identifier and URL rules

`id.Normalize` uppercases, removes hyphens, maps `I`/`L` to `1` and `O` to `0`, then requires exactly 26
characters from the Crockford alphabet. Reveal accepts only a full `http(s)://host[:port]/s/<id>` URL.
Burn additionally accepts a bare ID. Credentials, fragments, queries, escaped IDs, path aliases, and
trailing path segments fail before network access.

## Cryptography and phrases

The only supported envelope is:

```text
blob = 0x02 || salt(16) || iv(12) || AES-256-GCM(ciphertext || tag)
key  = PBKDF2-HMAC-SHA256(passphrase UTF-8, salt, 600000, 32 bytes)
AAD  = 0x02 0x02 || salt || iv
```

Generated phrases contain exactly seven distinct uniformly sampled words from the embedded 1,296-word EFF
Short Wordlist #2. Supplied phrases contain 7–64 distinct list members. Parsing accepts only ASCII
whitespace as separators, ASCII-lowercases accepted words, and joins them with one space. It never changes
a word or its order, and it never accepts a free-form escape. Raw typed commits, bracketed paste, plain-line
entry, and credential files/descriptors enforce that same word-count and whitespace grammar.

`burnerpad words` validates that it received no arguments, writes the EFF copyright and CC BY 3.0
attribution to stderr, and only then writes the exact 1,296-line canonical list to stdout. If attribution
cannot be written, it emits no wordlist bytes.

Every secret needs a unique phrase. Suite `0x02` does not authenticate the server-assigned ID, so phrase
reuse would allow a malicious server to substitute another valid blob created under the same phrase. The
generator makes a fresh random selection; supplied-phrase uniqueness is a caller responsibility.

The test harness hashes the exact raw upstream vector file against `VectorsSHA256`, then runs every applicable
suite-`0x02` decrypt, encrypt, and negative vector plus every generic encoding and encoding-negative vector.
A new applicable vector runs automatically, and a new error expectation fails until handled. Published PBKDF2
and Wycheproof primitives remain independent lower-level gates.

## Input ownership

Create plaintext comes from exactly one of the interactive composer, piped stdin, or `--input FILE`.
It is rejected before encryption/network if empty, invalid UTF-8, or larger than 65,491 bytes.

Passphrases come only from the controlling-terminal prompt, `--passphrase-file`, or `--passphrase-fd`.
Management tokens come only from a piped create receipt, controlling-terminal hidden prompt,
`--token-file`, or `--token-fd`. A valid management token is the canonical unpadded base64url encoding of
exactly 32 bytes and is therefore exactly 43 characters. File `-` is rejected for credentials and descriptors
must be at least 3.
Named credential files are opened once and validated on that opened handle before reading. Linux and macOS
require a regular file owned by the effective user with no group/other permission bits and refuse a symlink
as the final path component; macOS also refuses any extended ACL because it can grant access independently of
those mode bits. Windows opens the reparse point itself and requires a non-reparse regular disk
file owned by the process-token user; its protected, non-null DACL must contain a current-user allow entry and
no allow entry for another principal. Unknown Windows ACE forms fail closed. Policy failures are
`invalid_credential_source`; an inaccessible or missing file remains `local_io_failed`. Credential descriptor
options are deliberately excluded because an already-open descriptor is an explicit capability and may be a
pipe. Plaintext input, ciphertext input, and output/recovery destinations have separate policies.
No plaintext, passphrase, or token argv/environment interface exists. Interactive allocation is bounded at
the reader: bracketed paste and share-URL lines at 4,096 bytes, phrase lines at 1,024 bytes, and management
tokens at 256 bytes. An oversized line is wiped and drained before another prompt can consume input.

## Reveal lifecycle and recovery

Reveal performs fallible local work in this order:

1. parse and normalize the full URL;
2. collect and canonicalize the complete phrase;
3. reserve the selected plaintext and recovery files with exclusive owner-only creation;
4. preflight the selected file/terminal destination;
5. disclose the URL origin;
6. send one claim request;
7. immediately write an opted-in recovery blob;
8. decrypt and validate UTF-8 locally; and
9. deliver original plaintext to one byte-exact destination or a safe rendition to a terminal.

A valid wrong phrase may be retried locally by entering the complete phrase again against the held ciphertext;
the retry never triggers another claim.
`--keep-blob` writes canonical base64url plus one newline and retains it on success or failure. Without it,
abandoning held ciphertext warns, but neither stderr nor JSON ever receives the blob.

## Plaintext destinations

`--json` and `--out` are mutually exclusive. With no explicit destination, terminal stdout uses
one safe renderer in alternate-screen and plain/fallback modes. It preserves graphic UTF-8 and logical line
breaks while visibly escaping other control, format, and non-graphic characters; sender-controlled terminal
instructions never reach the TTY. Alternate-screen output is split into terminal-sized pages without copying
the complete rendered plaintext: Space or Enter advances, `b` goes back, and `q` closes. Terminals too small
for the frame use the scrollback fallback. The rendition is intentionally not byte-exact. Non-terminal stdout
and `--out` receive exact UTF-8 bytes, and decoding JSON's `plaintext` string reproduces them exactly. `--out`
creates mode `0600` (or an owner-only Windows DACL), uses exclusive creation, and never overwrites. A
reservation made before a failed claim is removed only by the invocation that created it. There is no built-in
clipboard destination: terminal clipboard protocols cannot confirm acceptance or completeness, and helper
executables would add a PATH/platform trust boundary. Callers may compose non-terminal stdout with their own
tool; destructive reveal should pair unverified external delivery with `--keep-blob`.

## Machine and exit contract

JSON uses four fixed success structs and one flat error struct. Error fields never contain a URL, ID,
phrase, token, ciphertext, plaintext, raw response, path, or nested error. `retry_after` appears only when a
server supplied a valid non-negative delta-seconds value; zero is valid. Field names and exit meanings are
documented in the README and ADR-0028. Exit `8` means only `unsupported_secret`: local ciphertext is not
canonical Burnerpad base64url or is not a supported passphrase envelope. A malformed final mutation response
cannot establish the operation's outcome and is therefore operation-specific exit `9`, not exit `8`.

One Run-owned cancellation context covers input, mutation transport, and interactive waits. On the first
signal, Run cancels and joins command dispatch before restoring the terminal or returning.
If cancellation leaves an already-transmitted mutation unconfirmed, its operation-specific outcome-unknown
result takes precedence. Definitive command/local failures found while joining also remain authoritative; a
confirmed success completes required post-success handoff, recovery, output, and cleanup attempts before the
signal result. Signal exits return 130/143 without synthesizing JSON. The production signal subscription is
stopped after the first signal so a second signal restores immediate OS termination. Exit zero means the
selected artifact reached its destination, not merely that crypto or HTTP succeeded. If confirmed one-time
plaintext becomes ready with cancellation already pending, the handoff cannot safely reuse that canceled
wait, so terminal delivery becomes persistent plain output. A signal arriving after an interactive delivery
began cancels that wait normally.

## Release and compatibility gates

`.burnerpad-lite-revision` is the reviewed current-server pin. Pull requests and releases run the reusable
`spec-drift` gate against that exact revision, byte-comparing the vendored specification, vectors, and
wordlist. They also run real Chromium interoperability in all three directions: browser→CLI, CLI→browser,
and CLI→CLI. The `lite-main-interop` workflow is configured to run the same interoperability tests against
Lite `main` on a schedule as an early drift warning.

GitHub Releases is the only official version-one binary source. A published version-one release will contain
the platform archives, native package files, archive SBOMs, checksum manifest, keyless Cosign checksum bundle,
and the release-rendered Linux/macOS installer. External package repositories, registries, and container
channels are deferred under ADR-0038.

The release version comes exclusively from the immutable `v*` Git tag via linker flags. Source contains no
release-version constant and a release requires no follow-up version-bump commit. The publisher builds and
checksums the tag artifacts, emits their SBOMs, signs the checksum manifest, and publishes them inside
GitHub's immutable-release boundary, which creates a release attestation. After publication,
`repro-verify` independently rebuilds every binary; announcement waits for that comparison.
