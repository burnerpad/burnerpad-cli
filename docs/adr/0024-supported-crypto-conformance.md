# ADR-0024: Gate conformance on the supported passphrase suite

Date: 2026-09-08 · Status: Accepted · Supersedes: [ADR-0003](0003-conformance-as-release-gate.md)

## Context

The shared language-neutral crypto specification defines suites `0x01` and `0x02`, but the current
burnerpad-lite product and both official clients use passphrase suite `0x02`. Keeping the fragment-key suite
in the Go CLI would expand its public behavior without serving the required browser/CLI interoperability or
any released compatibility obligation.

## Decision

The Go CLI implements and exposes only suite `0x02`. Its merge and release gates run every applicable
passphrase-suite known-answer, negative, encoding, and cross-implementation test against the pinned normative
crypto source. Suite `0x01` vectors remain valid for other implementations but are outside this CLI's
supported and advertised conformance surface.

## Consequences

- The crypto implementation, target parser, flags, prompts, documentation, and tests contain no fragment-key
  product path.
- The CLI reports the precise suite it supports instead of claiming conformance to the entire multi-suite
  specification.
- A future suite requires a new explicit product decision; it is not enabled merely because the shared
  specification defines it.

