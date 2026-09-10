# ADR-0005: One audited canonical-base64url decode gate

Date: 2026-08-18 · Status: Accepted · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §2 (M1), §11.4

## Context

SPEC §3 requires canonical base64url (one encoding per byte string) and *names Go's decoder as
insufficient*: `base64.RawURLEncoding` silently skips embedded `\r`/`\n` and accepts dirty trailing
bits; `.Strict()` fixes only the trailing bits — CR/LF are still skipped per the stdlib docs.
A lenient client would accept links a conformant client rejects: a cross-client compatibility bug in
a one-shot system.

## Decision

Every fragment and every API blob decode routes through one function, `envelope.DecodeCanonical`:
alphabet pre-scan → `len%4 == 1` reject → `Strict()` decode → **re-encode-and-compare** (the SPEC's
own conformance oracle). All failures are `reject_bad_encoding`. Nothing else in the codebase may
call `encoding/base64` decode functions — a grep check in CI enforces it. The function takes and
returns `[]byte` (never `string`) because for a 0x01 fragment the decoded output *is* the key.
Dedicated fuzz target: never panics, canonical bijection, agreement with the stdlib strict decoder.

## Consequences

- The Go pitfalls the SPEC warns about are closed in one audited place with vectors pinning each
  property (`enc-newline`, `enc-noncanon-*`, `b64url-tail-*`).
- Defense in depth: the pre-scan, the strict decode, and the re-encode each independently catch
  overlapping classes; changing stdlib leniency cannot loosen us.
