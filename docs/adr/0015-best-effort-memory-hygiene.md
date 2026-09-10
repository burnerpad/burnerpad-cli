# ADR-0015: Best-effort memory hygiene — `[]byte` secrets, wipe-on-defer, no memguard

Date: 2026-08-18 · Status: Accepted · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §2 (M4), §12

## Context

Go has a GC and no destructors; Rust-grade wipe-on-drop is unavailable (ADR-0001 accepted this).
The process lives ~200 ms and its secrets transit terminals and kernel buffers in every language.
Overselling hygiene would be dishonest; skipping it entirely widens windows needlessly.

## Decision

- Secrets are **`[]byte` end to end, never `string`** (grep-enforced in CI; the single sanctioned
  exception is the zero-copy `unsafe.String` alias inside `deriveKey` — the only `unsafe` in the
  codebase).
- `secret.Wipe` = `clear()` + `runtime.KeepAlive`; ownership rule: `envelope` wipes what it
  allocates (via `defer`, covering error paths); the `cmd` layer wipes what it passes in/receives.
- Startup hardening before any secret exists: `RLIMIT_CORE=0` everywhere; `PR_SET_DUMPABLE=0` on
  Linux; Windows documented as a residual gap (WER opt-out needs registry writes the CLI refuses).
- Best-effort `mlock`/`VirtualLock` on key/passphrase/plaintext buffers; failures ignored silently.
- `secret.Buffer` implements `Stringer`/`GoStringer` → `[redacted]`.
- **memguard rejected**: breaks the dependency freeze, cannot cover the dominant residual surface
  (stdlib-internal copies), and would advertise an illusion.

## Consequences

- A residual-copy table (AES round keys, HMAC ipad/opad, kernel buffers, swap, scrollback…) ships
  verbatim in SECURITY.md — the honest scope: narrowed windows, not erasure.
- Docs never claim "memory is scrubbed"; they claim exactly what is done.
