# ADR-0008: No config file, no persistent state, no telemetry, no update check

Date: 2026-08-18 · Status: Accepted · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §4.5, §18, §25

## Context

A secrets tool's most credible privacy claim is a mechanical one. Hidden state (a config file, a
cache, a version-check beacon) is exactly the surprise such a tool must not spring: a pinned
attacker server URL, a cached token, or an "this IP uses burnerpad" network beacon.

## Decision

The CLI is a pure function from (argv, env, stdin, one HTTPS exchange) to (stdout, stderr, exit
code). No config file, no dotfiles read, no XDG/AppData paths, no registry writes, no telemetry, no
crash reporting, no update check — not opt-out, **absent**. Disk is touched only by explicit
`--out`/`--keep-blob`/`--input`/`--*-file` flags and shell redirection. Configuration surface is
exactly: flags > environment (`BURNERPAD_SERVER`, `BURNERPAD_PLAIN`, plus standard
`NO_COLOR`/`CI`/`TERM`/proxy/cert vars) > built-in defaults. Passphrases and management tokens are never
read from environment variables.

## Consequences

- Self-hosters use `export BURNERPAD_SERVER=…` in their shell profile — visible, user-owned state.
- "No other host is ever contacted" is testable: the integration proxy asserts zero egress beyond
  the API host (release-QA invariant #4).
- Users learn of updates via package managers and the GitHub releases feed; a stdlib CVE triggers
  our own rebuild release (ADR-0001).
- The mgmt token is never persisted by the CLI; `--json > receipt.json` is the explicit, visible way
  to keep a receipt.
