# ADR-0028: Freeze the minimal machine interface for version one

Date: 2026-09-10 · Status: Accepted · Supersedes: [ADR-0007](0007-stream-discipline-and-machine-surface.md)

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

## Consequences

- Scripts can branch on stable categories without parsing human prose.
- The small error shape is safe to retain in CI logs and monitoring systems.
- Any incompatible change to these schemas or exit meanings requires a new major version.
