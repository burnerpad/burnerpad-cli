# ADR-0035: Retire unverifiable clipboard delivery before version one

Date: 2026-09-11 · Status: Accepted · Amends: [ADR-0028](0028-version-one-machine-interface.md), [ADR-0033](0033-explicit-plaintext-destinations.md)

## Context

Exit zero means the requested artifact reached its selected destination. OSC 52 can prove only that the CLI
wrote a terminal escape sequence: terminals may ignore it, impose unknown payload limits, or truncate a large
secret without acknowledgement. That uncertainty is unacceptable after a destructive one-time claim. Calling
the request “copied,” or weakening exit zero for one option, would make the machine contract misleading.

## Decision

Remove `--clip` from create, reveal, and decrypt, and remove OSC 52/tmux handling from the executable. The
built-in plaintext destinations remain the safe terminal viewer, exact non-terminal stdout, exclusive
owner-only files, and JSON. The CLI will not execute native clipboard helpers because that would add
PATH-resolved code and platform-specific trust boundaries contrary to ADR-0002.

A caller may deliberately pipe exact stdout to a clipboard program it selects. That composition and any
clipboard retention are caller-owned; destructive reveal should use `--keep-blob` when an external handoff
may need recovery.

## Consequences

- Exit zero keeps one unqualified meaning for every built-in destination.
- Silent terminal rejection, truncation, delayed clearing, and clearing of a newer clipboard value disappear
  from the CLI's security surface.
- SSH/tmux clipboard convenience and timed best-effort clearing are no longer built in.
- The incompatible flag removal is made before the first release, as permitted by ADR-0030.

Requiring `--keep-blob` while retaining OSC 52 was rejected: it protects recovery but still cannot establish
clipboard delivery. Native helper APIs and executables were rejected because they enlarge the implementation
and trust boundary without making a remote terminal clipboard verifiable.
