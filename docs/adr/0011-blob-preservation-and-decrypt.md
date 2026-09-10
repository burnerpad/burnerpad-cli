# ADR-0011: Blob preservation after the burn; `decrypt` as the offline recovery half

Date: 2026-08-18 · Status: Superseded by [ADR-0027](0027-explicit-claimed-blob-recovery.md) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §7.3, §7.4

This record automatically exposed ciphertext through stderr and JSON after a failed reveal. Recovery remains
available under the current design, but only through a destination explicitly selected before the claim.


## Context

After the burning GET, this process's RAM holds the only copy of the ciphertext in the universe.
A wrong passphrase, a Ctrl+C, or a crash-path exit that discards it silently destroys the last copy
of a secret the sender believes was delivered.

## Decision

**The process never exits post-fetch — by error, wrong passphrase, or Ctrl+C — without preserving
the ciphertext.** The envelope is safe to display (without the phrase/key it is as useless to an
observer as the server's copy was):

- Wrong phrase → unlimited **local** retry against the in-RAM blob; committed words retained for
  editing; no attempt counter (the security bound is PBKDF2 × phrase entropy, not UI throttling).
- First post-fetch Ctrl+C warns; the second prints the sealed envelope (canonical base64url) plus a
  ready-to-run recovery command to stderr, then exits 130.
- Non-interactive failure: envelope to stderr when stderr is a TTY; always in the `--json` error
  object (`"blob"`); `--keep-blob FILE` writes it (ciphertext only, never key material).
- **`burnerpad decrypt`** is the recovery half: local-only (no network I/O), reads a raw or
  base64url envelope, runs the same prompt/decrypt machinery, and doubles as the vector-harness
  entry point. Suite mismatches post-fetch (fragment given but blob is 0x02, or vice versa) pivot to
  the right credential flow with the blob already safe in RAM.

## Consequences

- Signal handlers must complete the preservation print before exiting (termios restored → buffers
  wiped → print → exit 128+n) — tested, not hoped.
- Scripts can branch on exit 5 with `.error.blob` and retry offline later.
