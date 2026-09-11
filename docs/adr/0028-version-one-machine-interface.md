# ADR-0028: Freeze the minimal machine interface for version one

Date: 2026-09-10 · Status: Accepted · Amended by: mutation-aware signals, [ADR-0035](0035-retire-unverifiable-clipboard-delivery.md), [ADR-0036](0036-retire-inert-quiet-option.md), [ADR-0041](0041-retire-unreachable-invalid-server-response.md) · Supersedes: [ADR-0007](0007-stream-discipline-and-machine-surface.md)

ADR-0035 removes clipboard delivery from the local-failure and pending-cancellation portions of this interface;
the remaining machine contract is unchanged.

## Amendment: quiet-mode retirement (2026-09-11)

[ADR-0036](0036-retire-inert-quiet-option.md) removed the unreleased `--quiet` option, which never affected
output, before version one. Required recovery data, warnings, and diagnostics remain unconditional; the original
quiet-mode sentence below is retained as history.

## Amendment: unreachable invalid-response error retirement (2026-09-11)

[ADR-0041](0041-retire-unreachable-invalid-server-response.md) removes the unreleased
`invalid_server_response` code and its unreachable API error type. Exit `8` now means only
`unsupported_secret`; the broader exit meaning in the original decision below is retained as history.

## Context

The old CLI's unreleased JSON shapes and exit meanings described obsolete endpoints, suites, and recovery
behavior. Version one needs a small interface that lets automation distinguish safe local failures, a
definitively unavailable secret, and a destructive request whose outcome cannot be known, without copying
secret material into diagnostics.

## Decision

Successful `--json` operations emit exactly one of these objects on stdout:

```json
{"status":"created","server":"https://burnerpad.io","link":"…","phrase":"…","mgmt_token":"…","ttl":86400}
{"status":"revealed","server":"https://burnerpad.io","plaintext":"…"}
{"status":"burned","server":"https://burnerpad.io"}
{"status":"decrypted","plaintext":"…"}
```

Machine-readable failures use this shape, with `retry_after` added only when the server supplies one:

```json
{"status":"error","code":"claim_outcome_unknown","message":"…","server":"https://burnerpad.io"}
```

Error objects never contain a URL, identifier, phrase, management token, ciphertext blob, plaintext, raw
response, or nested error. A local-only failure may omit `server` when no server was selected. Human-mode
stdout contains only the operation artifact; prompts, warnings, and diagnostics use the controlling terminal
or stderr. Quiet mode never hides information required to recover a created secret. The user-facing command
for revocation is `burn`; there is no `revoke` command or alias.

Version one freezes these exit statuses:

| Status | Meaning |
|---:|---|
| `0` | Success |
| `2` | Invalid command, option, input, or credential source |
| `3` | Local file, terminal, clipboard, or output failure |
| `4` | Secret unavailable |
| `5` | Passphrase authentication failed or authenticated plaintext is invalid |
| `6` | Server definitively rejected the operation |
| `7` | Rate limited or temporarily unavailable |
| `8` | Invalid or incompatible server response |
| `9` | Mutation outcome unknown |
| `10` | Internal failure |
| `130`, `143` | Process terminated by the corresponding signal |

The first SIGINT or SIGTERM cancels and joins the active command. If cancellation leaves an
already-transmitted mutation unconfirmed, the operation-specific exit-`9` result takes precedence. A
definitive command/local failure found while joining remains authoritative; a confirmed success completes its
required handoff before the process returns `130` or `143`. Signal exits emit no ordinary JSON error. A second
production signal restores the operating system's immediate termination behavior. If confirmed one-time
plaintext becomes ready with cancellation already pending, terminal handoff uses persistent plain rendering
and clipboard handoff completes its configured dwell instead of immediately erasing the destination.

## Consequences

- Scripts can branch on stable categories without parsing human prose.
- The small error shape is safe to retain in CI logs and monitoring systems.
- Any incompatible change to these schemas or exit meanings requires a new major version.
