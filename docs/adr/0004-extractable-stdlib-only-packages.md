# ADR-0004: `envelope/` and `wordlist/` as public, extractable, stdlib-only packages

Date: 2026-08-18 · Status: Accepted · Amended by: [ADR-0024](0024-supported-crypto-conformance.md) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §11, §13, §19

## Context

The SPEC anticipates "an SDK in another language". The parent extracted `@burnerpad/crypto` into its
own repo; the Go implementation should be liftable the same way, growing the auditor pool.

## Decision

`envelope/` (the supported spec-v1 passphrase suite, DecodeCanonical, PBKDF2, and its applicable errors) and `wordlist/`
(EFF list, phrase generation) are **public packages at the repo root**: zero imports outside the
stdlib, no imports from `internal/`, no I/O, no policy, no logging. `envelope` defines its own
private 4-line `wipe()` rather than importing anything. Package-boundary rule CI-enforced via a
`go list -deps` assertion: `internal/api` imports `envelope` only for error types; `internal/term`
imports `wordlist` and `x/term`; `internal/cli` alone imports everything.

## Consequences

- Either package can become its own module ("the Go SDK") without surgery.
- Crypto failures impossible under supported spec parameters **panic** rather than growing the documented
  surface (the fuzzer asserts the closed surface).
- The wordlist's licensing (CC BY 3.0) rides with the package via embedded attribution (ADR-0018).
