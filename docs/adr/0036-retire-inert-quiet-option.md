# ADR-0036: Retire the inert quiet option before version one

Date: 2026-09-11 · Status: Accepted · Amends: [ADR-0025](0025-operation-authoritative-server-origin.md), [ADR-0028](0028-version-one-machine-interface.md)

## Context

The unreleased CLI parsed and documented `--quiet` as “less decoration,” but no output path consulted the
setting. The interface therefore accepted a promise with no defined or observable effect. The output that
could plausibly be hidden—server disclosure, safety warnings, diagnostics, and create recovery data—is
required for auditability or recovery and must remain visible.

## Decision

Remove `--quiet` from configuration, parsing, help, completions, and active documentation before version one.
Supplying it is an invalid option. Human-mode required output remains unconditional; callers that need a
single machine result use `--json`.

## Consequences

- The advertised interface contains no behavior-free compatibility surface.
- Scripts with a stale `--quiet` fail visibly at exit 2 instead of appearing to request behavior they do not
  receive.
- The incompatible removal is made before the first release, as permitted by ADR-0030.

Defining and implementing a new decoration taxonomy was rejected because it would add branches without
improving secrecy, recovery, or automation. Silently retaining the no-op was rejected because accepted but
ineffective options make the CLI harder to understand and future compatibility harder to reason about.
