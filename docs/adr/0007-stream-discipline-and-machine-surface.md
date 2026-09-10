# ADR-0007: Stream discipline, `--json`, and exit codes as a compatibility promise

Date: 2026-08-18 · Status: Superseded by [ADR-0028](0028-version-one-machine-interface.md) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §5.1, §8.3, §9

This record froze schemas and exit meanings designed around the retired service contract. ADR-0028 replaces
that unreleased surface with the smaller current-contract interface used by the first public release.

## Context

`cat .env | burnerpad create` must feel like it was always a Unix tool. Scripts need to branch on
the distinctions that matter (retryable? gone? ambiguous burn?) without parsing prose.

## Decision

- **stdout carries exactly one thing — the artifact** (URL on create, raw plaintext on reveal, the
  JSON object with `--json`). Everything else — prompts, phrase display, mgmt token, warnings,
  errors — goes to stderr or the controlling terminal.
- Interactivity is decided **only** by `term.IsTerminal` on the relevant fd.
- The `--json` schemas (§8.3) and the exit-code table (§9: 0–11, 130/143) are **frozen v1 surface**,
  documented in the man page. One table in `internal/cli/exit.go` is the single source of truth.
- The stderr diagnostic grammar is stable and grep-safe: `burnerpad: <category>: message`.

## Consequences

- `--quiet` may suppress decoration but never stderr *artifacts* (phrase, mgmt token, suite
  announcement) — suppressing those would mint unopenable secrets; with `--json` they move into the
  JSON object instead.
- Changing any promised surface is a major version (expected: never).
- Golden tests pin every JSON shape and every fixed human message.
