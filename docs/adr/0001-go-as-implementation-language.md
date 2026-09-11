# ADR-0001: Go as the implementation language (floor go1.25)

Date: 2026-08-18 · Status: Accepted · Amended by: [ADR-0039](0039-two-direct-go-team-modules.md) (direct dependency count) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §3

## Context

The CLI is a confidentiality tool: it needs audited crypto + TLS, static cross-platform binaries,
a tiny auditable dependency graph, and a contributor pool able to read the whole ~2 kLOC in one
sitting. The parent project's ethos is extreme minimalism (1-dep server, 0-dep crypto lib).
Candidates surveyed: Go, Rust, and Zig (the only credible third from a wider field).

## Decision

Implement in **Go**, `go 1.25` as the `go.mod` floor, with releases built on the latest supported stable
toolchain patch. Go 1.24 supplied the required stdlib cryptography, while the selected `x/term` generation
raises the actual module floor to Go 1.25; the module declaration is the authoritative compatibility claim.

## Rationale

Go is the only candidate where audited, FIPS-capable crypto **and** TLS **and** HTTP **and** JSON are
all first-party, leaving exactly one external module (`x/term`). It wins single-host
cross-compilation of all six targets, static libc-free Linux binaries, native fuzzing, and auditor
reach. Weighted decision matrix in ARCHITECTURE.md §3 (Go 8.13, Rust 7.29, Zig 7.18).

## Consequences

- Rust's wins are forfeited and must be mitigated: parser strictness → the M1 decode gate
  (ADR-0005); memory hygiene → best-effort posture, documented honestly (ADR-0015).
- ~7 MiB binaries accepted; a ≤ 9 MiB CI gate guards drift.
- A Go stdlib security fix in `crypto/*`/`net/http`/`encoding/*` **is** our security release
  (rebuild ≤ 72 h).

## Alternatives considered

- **Rust**: wins correctness-by-construction, `zeroize`, binary size — but a realistic build carries
  a 60–80-crate lockfile, dissonant with a project advertising one dependency.
- **Zig**: zero-dep purity purchased with an unaudited homegrown TLS client — disqualifying here.
