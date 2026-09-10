# ADR-0003: Envelope-spec conformance as a merge and release gate

Date: 2026-08-18 · Status: Superseded by [ADR-0024](0024-supported-crypto-conformance.md) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §2.1 (M2), §14

This record gated the unreleased CLI on every suite in the shared envelope specification. The current Go CLI
supports only the passphrase suite used by burnerpad-lite's official clients and gates the corresponding
conformance surface instead.


## Context

The CLI's whole value is that a link minted anywhere opens everywhere. The spec repo ships
language-neutral vectors (`vectors/v1.json`: 7 decrypt KATs, 6 encrypt KATs, 25 negatives, 3+6
encoding cases) and declares: conformance = green on the entire set.

## Decision

- Vendor `vectors/v1.json` **verbatim**, sha256-pinned in the test
  (`6b0faefe49abf324c42ca5d2c682f2a072039b95588d65571072ab6f490bafaf`); the harness hard-fails if
  the vendored bytes change.
- The full set runs on every push; `release.yml` re-runs it at the gate — **a red vector can neither
  merge nor ship, mechanically**.
- Encrypt KATs replay fixed randomness through the unexported `randRead` seam (white-box,
  same-package) so no fixed-IV public entrypoint ever ships.
- A `spec-drift` CI job (push + weekly) byte-compares vendored SPEC/vectors/wordlist against the
  pinned upstream tag.
- Weekly + pre-release **differential testing** against the JS reference closes the Go↔JS loop
  (verdict + reason agreement on single-fault cases; verdict-only on multi-fault garbage — v1 pins
  no reject precedence).

## Consequences

- The five canonical reject strings are the error values themselves (`envelope.Error`), so there is
  no mapping layer to drift.
- The CLI's §11.3 check ordering is pinned by fuzz-corpus determinism even though scripts can't
  observe it (exit 6 groups all four structural rejects).
- Upstream vector evolution turns CI red instead of silently skipping classes (strict loader).
