# ADR-0002: One-dependency freeze; hand-rolled terminal layer

Date: 2026-08-18 · Status: Accepted · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §2 (M6), §7.2, §19

## Context

Every dependency is supply-chain surface and audit burden. The parent advertises "1-dep server /
0-dep crypto lib". The CLI needs raw terminal mode (autocomplete, prompts) — the usual Go answer is
a TUI framework (Bubble Tea) and cobra for flags, each dragging tens of modules.

## Decision

Direct dependencies = **`golang.org/x/term` only** (transitive: `x/sys`), both Go-team-maintained
and sumdb-pinned. CI fails if `go.mod` grows — including test-only deps. Consequences embraced
explicitly:

- the list-locked autocomplete is **hand-rolled** (~300 lines over raw mode) as a pure state machine;
- flag parsing is stdlib `flag` with per-subcommand FlagSets and a ~15-line re-parse loop for
  GNU-style interleaving (no cobra);
- shell completions are static, hand-written, `go:embed`-ed scripts;
- the Linux PTY smoke test hand-rolls `/dev/ptmx` over `x/sys` ioctls instead of `creack/pty`.

## Consequences

- The import graph is CI-asserted (`go list -deps` allowlist) and paired with the ≤ 9 MiB size gate.
- More first-party code to test (the state machine gets its own exhaustive unit matrix) in exchange
  for a graph an auditor can hold in their head.
- `memguard`, clipboard helper binaries, and any TUI framework are rejected by this rule alone.
