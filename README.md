# Burnerpad CLI

The official command-line client for [burnerpad.io](https://burnerpad.io) and current
burnerpad-lite servers. It creates, claims, revokes, and locally decrypts end-to-end-encrypted,
one-time text secrets that interoperate with the browser client.

The CLI supports the current passphrase-only product: suite `0x02`, seven or more distinct words
from the shared EFF wordlist, and 26-character Crockford secret identifiers. It intentionally does
not support pre-release fragment links, suite `0x01`, retired endpoints, or old short IDs.

## Installation

[GitHub Releases](https://github.com/burnerpad/burnerpad-cli/releases) is the only official binary
distribution source. If that page has no release, Burnerpad has not shipped yet. The `install.sh` in this
source tree is a refusing release template, not a usable installer.

For the latest stable Linux or macOS release on amd64 or arm64, download the release-rendered,
checksum-pinned script, read it, then have it verify the matching archive before installing:

```sh
curl --proto '=https' --proto-redir '=https' -fL -o burnerpad-install.sh \
  https://github.com/burnerpad/burnerpad-cli/releases/latest/download/install.sh
cat burnerpad-install.sh
sh burnerpad-install.sh --verify-only
sh burnerpad-install.sh
```

The script uses no `sudo` and installs to `~/.local/bin` by default. Set `BURNERPAD_INSTALL_DIR` to choose
another directory. The script itself is the trust root for its embedded archive checksum. Prereleases must
be downloaded from their exact tag page rather than the `latest` URL.

When a release exists, its Windows archives appear on the Releases page, but there is no Windows installer.
Download the matching `.zip`, `SHA256SUMS`, and `SHA256SUMS.bundle`; perform
[release verification](SECURITY.md#release-verification) with Cosign and `sha256sum` from Git Bash or WSL;
then extract `burnerpad.exe` to a directory on `PATH`.

For source evaluation, a checkout can be built with Go 1.25 or newer:

```sh
git clone https://github.com/burnerpad/burnerpad-cli.git
cd burnerpad-cli
make build
./burnerpad version
```

## Quick start

Create a secret using the default `https://burnerpad.io` server:

```sh
burnerpad create
```

For automation, request the stable JSON receipt. It contains the newly generated random phrase and must be
handled as secret material:

```sh
printf '%s' 'database password' |
  burnerpad create --json
```

Create against a self-hosted loopback server:

```sh
printf '%s' 'database password' |
  burnerpad create --server http://127.0.0.1:4000 --json
```

Every network operation identifies the server it will contact. `create` and bare-ID `burn` use
`--server`, then `BURNERPAD_SERVER`, then `https://burnerpad.io`. A full share URL always selects its
own origin. `reveal --server ...` is accepted only to warn that the option is ignored.

Reveal a secret after placing its separately received, one-time phrase in an owner-only file named `phrase`:

```sh
burnerpad reveal --passphrase-file phrase 'https://burnerpad.io/s/0123456789ABCDEFGHJKMNPQRS'
```

A URL on the command line is accepted with a shell-history warning. To avoid that, pipe only the
URL or paste it into the protected terminal prompt:

```sh
printf '%s\n' "$BURNERPAD_LINK" | burnerpad reveal --passphrase-file phrase
```

Burn without revealing by piping the JSON create receipt:

```sh
burnerpad create --json --passphrase-file phrase < secret.txt |
  burnerpad burn --json
```

Named passphrase and management-token files are checked, not merely described as protected. On Linux and
macOS the opened object must be a regular file owned by the current user with no group or other permission
bits (normally mode `0600` or `0400`), no macOS extended ACL, and no symlink as its final path component. On
Windows it must be
a non-reparse regular disk file owned by the current user, with inheritance disabled and a DACL that grants
access only to that user. An unsafe file is rejected before network access. File descriptors are already-open
capabilities and may intentionally be pipes, so these checks apply only to `--passphrase-file` and
`--token-file`. Omit the file option to use the protected terminal prompt if managing file permissions is
inconvenient.

## Commands

| Command | Purpose |
|---|---|
| `create` | Encrypt UTF-8 text locally and upload one opaque suite-`0x02` blob |
| `reveal` | Claim one blob exactly once, then decrypt and display it locally |
| `burn` | Revoke a secret using its management token without reading it |
| `decrypt` | Decrypt a previously preserved blob without network access |
| `words` | Attribute the EFF list on stderr, then print its 1,296 words on stdout |
| `completion` | Print completion for Bash, Zsh, Fish, or PowerShell |
| `version` | Print build, crypto, wordlist, and server-contract identity |
| `licenses` | Print embedded license and attribution notices |
| `help` | Print general or command-specific help |

There are no command aliases and no implicit reveal. Run `burnerpad help <command>`,
`burnerpad <command> --help`, or `burnerpad <command> -h` for the exact options.

### Create

```text
burnerpad create [--server ORIGIN] [--ttl DURATION] [--input FILE]
                 [--ask | --passphrase-file FILE | --passphrase-fd FD]
                 [--json]
```

Plaintext comes from exactly one source: the interactive composer, piped stdin, or `--input FILE`.
It must be non-empty, valid UTF-8, and no larger than 65,491 bytes. A TTL must be positive whole
seconds; forms such as `90`, `90s`, `15m`, and `4h` are accepted. The server's returned effective
TTL is authoritative and the CLI reports any clamp.

With no supplied phrase, `create` generates exactly seven distinct uniformly sampled words. A
caller-supplied phrase must contain 7–64 distinct shared-list words. ASCII whitespace and case are
accepted and canonicalized to lowercase words separated by one space. A supplied phrase must also be unique
to that secret: reusing a suite-`0x02` phrase permits a malicious server to substitute another valid secret
encrypted under the same phrase. Interactive phrase lines are bounded at 1,024 bytes; oversized lines and
pastes are rejected without being echoed and are wiped after rejection.

For a successful non-JSON create, stdout contains only the share link. Stderr identifies the server and then
reports the management token and effective TTL; it also reports a generated phrase. When plaintext is piped
and the caller supplied the phrase separately, the CLI does not repeat that phrase. Piped plaintext with a
generated phrase requires `--json` so automation receives the complete handoff receipt.

### Reveal

```text
burnerpad reveal [--ask | --passphrase-file FILE | --passphrase-fd FD]
                 [--keep-blob FILE]
                 [--out FILE | --json]
                 FULL_SHARE_URL
```

Reveal accepts a full `/s/<id>` HTTP(S) URL, never a bare ID. It validates the URL, passphrase, and
all output destinations before making one `POST /api/secrets/:id/reveal` request. It never retries a
claim automatically. A wrong valid phrase can be corrected locally against the already-held blob.

`--keep-blob FILE` reserves a new mode-`0600` file before the claim and writes canonical unpadded
base64url ciphertext immediately after the claim. The recovery file remains on success or failure.
Without it, ciphertext is never printed in an error or JSON response.

### Burn

```text
burnerpad burn [--server ORIGIN] [--token-file FILE | --token-fd FD]
               [FULL_SHARE_URL | ID]
```

Burn accepts either a piped create receipt, a full URL plus a protected token source, or a bare ID
plus a protected token source. Without a token file or descriptor it prompts without echo. Tokens
are never accepted through argv or environment variables. Identifier input is ASCII-only; lowercase
letters and hyphens are accepted, with Crockford `I`/`L` folded to `1` and `O` folded to `0`.

### Offline decrypt

```text
burnerpad decrypt --blob-file FILE|-
                  [--ask | --passphrase-file FILE | --passphrase-fd FD]
                  [--out FILE | --json]
```

The blob format is canonical unpadded base64url with one optional terminal newline—the exact format
written by `--keep-blob`. Offline decrypt never constructs an HTTP client.

## Output contract

Without a destination option, reveal/decrypt show a terminal-safe rendition on TTY stdout, using the
alternate screen when available and the same renderer in `--plain`/fallback mode. Graphic UTF-8 stays
readable, LF/CRLF remain line breaks, and other control, format, and non-graphic characters are shown as
inert Go-style escapes such as `\t`, `\x1b`, and `\u202e`. The alternate-screen viewer pages content to
the current terminal: Space or Enter advances, `b` goes back, and `q` closes. If the terminal is too small
for a safe page frame, the viewer uses the scrollback fallback instead. This display is intentionally not
byte-exact.
Piped stdout and `--out` receive the original authenticated UTF-8 bytes; decoding JSON's `plaintext`
string reproduces those bytes exactly. `--out` creates a new owner-only file and never overwrites; `--json`
and `--out` are mutually exclusive. Burnerpad has no built-in clipboard destination because terminal
clipboard protocols cannot verify acceptance or completeness. A caller may pipe byte-exact stdout to its
own clipboard tool, but owns that tool and its retention behavior; use `--keep-blob` before a destructive
reveal when that external delivery may need recovery.

Stable JSON successes are:

```json
{"status":"created","server":"https://burnerpad.io","link":"…","phrase":"…","mgmt_token":"…","ttl":86400}
{"status":"revealed","server":"https://burnerpad.io","plaintext":"…"}
{"status":"burned","server":"https://burnerpad.io"}
{"status":"decrypted","plaintext":"…"}
```

Errors are flat and secret-free:

```json
{"status":"error","code":"claim_outcome_unknown","message":"…","server":"https://burnerpad.io"}
```

`retry_after` is the only optional field, and appears only when the server supplies a valid non-negative
delta-seconds value; zero is valid. Ordinary JSON errors use the following closed code vocabulary; no code is
emitted for success or signal exits.

| Exit | JSON error codes | Meaning |
|---:|---|---|
| `0` | — | Requested artifact reached its destination |
| `2` | `invalid_command`, `invalid_option`, `invalid_input`, `invalid_credential_source` | Invalid command, option, input, or credential source |
| `3` | `local_io_failed` | Local terminal, file, or output failure |
| `4` | `secret_unavailable` | Secret unavailable |
| `5` | `passphrase_failed`, `plaintext_invalid` | Passphrase failed or authenticated plaintext was not UTF-8 |
| `6` | `server_rejected` | Server definitively rejected the request |
| `7` | `network_unavailable`, `rate_limited`, `service_unavailable` | Network, rate-limit, or temporary service failure |
| `8` | `unsupported_secret` | Unsupported ciphertext |
| `9` | `create_outcome_unknown`, `claim_outcome_unknown`, `revoke_outcome_unknown` | A mutation may have happened, but its outcome is unknown |
| `10` | `internal` | Internal failure |
| `130`, `143` | — | SIGINT or SIGTERM |

The first SIGINT or SIGTERM cancels outstanding work but does not abandon it. Burnerpad waits for a mutation
request to be classified and for required handoff and cleanup attempts to finish. If cancellation leaves an
already-transmitted mutation unconfirmed, its operation-specific outcome-unknown error (exit `9`) takes
precedence. A definitive command/local failure also remains authoritative; a confirmed success completes its
required handoff and then returns `130`/`143`. Signal exits do not emit a JSON error object. Interrupting a
viewer restores the terminal before exiting. If cancellation was already pending when confirmed one-time
plaintext became ready, terminal delivery uses persistent plain rendering instead of an alternate-screen
wait. A second signal restores the operating system's immediate termination behavior and can therefore bypass
best-effort cleanup.

## Security model

The server receives only opaque ciphertext. Share links contain no decryption key; the passphrase
travels separately. Remote origins require HTTPS, loopback development may use HTTP, redirects are
not followed, and there is no TLS-verification bypass. Create, claim, and revoke each issue at most
one request per invocation.

Passphrases and management tokens have no argv or environment interface. The CLI has no config file,
persistent state, telemetry, update check, or additional network probe. Go memory wiping and page
locking are best effort; endpoint compromise, keyloggers, shell history, clipboard history, and
terminal scrollback remain outside the CLI's control. Plain mode can retain the safe rendition in
scrollback. A caller that sends byte-exact pipe, file, or decoded-JSON output to a terminal assumes the risk
of interpreting its controls. See [SECURITY.md](SECURITY.md).

## Build and test

Go 1.25 or newer is supported; release builds use the toolchain patch pinned in `go.mod`.

```sh
make build
make test
make lint
```

CI pins the raw upstream vector file, executes the complete applicable suite-`0x02` and generic encoding
vectors, byte-checks vendored crypto material, runs six cross-builds, and exercises real Chromium
interoperability with the reviewed burnerpad-lite revision. The `lite-main-interop` workflow is configured
to run the same browser/CLI matrix on a schedule against burnerpad-lite `main` to detect future drift.

See [CONTRIBUTING.md](CONTRIBUTING.md), [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md), and
[RELEASING.md](RELEASING.md).
