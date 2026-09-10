burnerpad(1)

# NAME

burnerpad - encrypted, one-time text secrets compatible with burnerpad-lite

# SYNOPSIS

*burnerpad create* [*--server* _ORIGIN_] [*--ttl* _DURATION_] [*--input* _FILE_]
[*--ask* | *--passphrase-file* _FILE_ | *--passphrase-fd* _FD_] [*--json*] [*--clip*]

*burnerpad reveal* [*--ask* | *--passphrase-file* _FILE_ | *--passphrase-fd* _FD_]
[*--keep-blob* _FILE_] [*--out* _FILE_ | *--clip*[=_DURATION_] | *--json*] _FULL_URL_

*burnerpad burn* [*--server* _ORIGIN_] [*--token-file* _FILE_ | *--token-fd* _FD_]
[_FULL_URL_ | _ID_]

*burnerpad decrypt* *--blob-file* _FILE_|- [*--ask* | *--passphrase-file* _FILE_ |
*--passphrase-fd* _FD_] [*--out* _FILE_ | *--clip*[=_DURATION_] | *--json*]

*burnerpad* { *words* | *completion* _SHELL_ | *version* | *licenses* | *help* [_COMMAND_] }

# DESCRIPTION

burnerpad encrypts UTF-8 text locally using the current passphrase-only suite 0x02, uploads only opaque
ciphertext, and interoperates with the current burnerpad-lite browser. A secret is removed by the server
when one claim request succeeds. The CLI never retries create, claim, or revoke automatically.

The default server is _https://burnerpad.io_. Network commands identify the selected origin before sending.
Create and bare-ID burn use *--server*, then _BURNERPAD_SERVER_, then the default. Reveal always uses the
origin in its full share URL; an explicit *--server* is ignored with a warning.

# COMMANDS

*create*
	Read plaintext from the interactive composer, piped stdin, or *--input*. The text must be non-empty,
	valid UTF-8, and at most 65,491 bytes. Generate seven distinct random words unless one protected
	passphrase source is supplied. A piped create with a generated phrase requires *--json*.

*reveal*
	Accept only a full _http(s)://host[:port]/s/id_ URL. Validate the phrase and destinations before one
	claim. Wrong-phrase correction is local against held ciphertext. *--keep-blob* saves canonical
	base64url ciphertext immediately after claim for offline recovery.

*burn*
	Revoke without reading. Input is a piped create JSON receipt, a full URL, or a bare current ID. The
	management token comes from the receipt, protected file/fd, or a hidden controlling-terminal prompt.

*decrypt*
	Decrypt a canonical base64url recovery blob locally. This command creates no HTTP client.

*words*, *completion*, *version*, *licenses*, *help*
	Print purpose-specific utility output. Completion supports bash, zsh, fish, and powershell.

# OPTIONS

*--server* _ORIGIN_
	Server origin for create or bare-ID burn. HTTPS is required remotely; HTTP is accepted only on loopback.

*--timeout* _DURATION_
	Whole-operation HTTP deadline. Default: 12s.

*--json*
	Emit one stable JSON success or error object. Mutually exclusive with *--out* and *--clip*.

*--quiet*
	Suppress decoration. Security warnings, server disclosure, and necessary create recovery data remain.

*--plain*
	Use accessible line prompts and avoid raw-mode/alternate-screen behavior.

*--no-color*
	Disable color. _NO_COLOR_ and _TERM=dumb_ are also honored.

*--ttl* _DURATION_
	Optional positive whole seconds, as an integer or a duration such as 15m. The server-returned effective
	TTL is authoritative and any clamp is reported.

*--input* _FILE_
	Read create plaintext from a named file. It conflicts with piped stdin.

*--ask*, *--passphrase-file* _FILE_, *--passphrase-fd* _FD_
	Select exactly one passphrase source. Files must be named and descriptors at least 3. There is no argv,
	stdin, or environment passphrase interface.

*--token-file* _FILE_, *--token-fd* _FD_
	Select one management-token source. With neither, burn uses a hidden controlling-terminal prompt.

*--keep-blob* _FILE_
	Reserve a new owner-only recovery file before claim and retain it after every post-claim outcome.

*--out* _FILE_
	Write plaintext to a newly created owner-only file. Existing paths are never overwritten.

*--clip*[=_DURATION_]
	Use OSC 52 instead of display. Reveal/decrypt default to a 45s best-effort clear; clipboard managers may
	retain history. Create copies only its share link.

# PASSPHRASES

Phrases contain 7-64 distinct members of the embedded EFF Short Wordlist #2. ASCII whitespace and case are
accepted and canonicalized to lowercase words joined by one space. Off-list and duplicate words fail before
network access. Generated phrases contain exactly seven uniformly sampled distinct words. Never reuse a
phrase across secrets; suite 0x02 does not bind the server-assigned ID into its authenticated data.

# OUTPUT

With no explicit destination, reveal/decrypt use a terminal-safe escaped rendition on terminal stdout and
exact UTF-8 bytes on piped stdout. The same safe renderer applies in plain/fallback mode. Network JSON results
include _server_; decoding their _plaintext_ string reproduces the authenticated UTF-8 bytes. Errors are flat
and never contain the URL, ID, phrase, management token, ciphertext, plaintext, raw response, path, or nested
error.

# EXIT STATUS

0 success; 2 invalid command/option/input/credential source; 3 local I/O/terminal/clipboard failure;
4 secret unavailable; 5 passphrase or plaintext validation failure; 6 definitive server rejection;
7 network/rate-limit/temporary service failure; 8 invalid server response or unsupported ciphertext;
9 mutation outcome unknown; 10 internal failure; 130 SIGINT; 143 SIGTERM.

# ENVIRONMENT

_BURNERPAD_SERVER_
	Server for create and bare-ID burn when *--server* is absent.

_BURNERPAD_PLAIN_, _NO_COLOR_, _TERM_, _CI_
	Prompt and presentation controls. No environment variable may contain plaintext, passphrases, or tokens.

# SECURITY

Share links contain no key. Send the link and phrase over separate channels. A URL supplied in argv is
accepted with a shell-history warning. There is no redirect following, TLS-verification bypass, telemetry,
configuration file, update check, or runtime server probe. Memory wiping and clipboard clearing are best
effort; see _SECURITY.md_ for limits.

# SEE ALSO

https://burnerpad.io
