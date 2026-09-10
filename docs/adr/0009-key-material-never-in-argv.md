# ADR-0009: Key material never in argv, requests, or logs — refusal by absence

Date: 2026-08-18 · Status: Superseded by [ADR-0031](0031-explicit-commands-and-protected-inputs.md) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §2 (M5), §4.2, §7.5, §17

This record permitted fragment-bearing links, environment passphrases, and argv management tokens. The
current product has no fragment-key suite, and ADR-0031 replaces every credential source before version one.

## Context

argv is visible in `/proc/*/cmdline` and persisted in shell history. The SPEC's rule for the
fragment is absolute: "a CLI must never log it". But users paste fragment-bearing URLs anyway, and
refusing after the history entry exists helps nobody.

## Decision

- There is **no `--passphrase <words>` or `--key <b64>` flag in the grammar at all** — refusal by
  absence; the unknown-flag error names the safe alternatives. A phrase detected as a second
  positional is refused (exit 2) with history-scrub advice.
- A **pasted fragment-bearing URL is accepted** (the damage is done): the fragment is split off into
  a wipeable buffer before anything else, requests are built from the id alone, and a one-time
  stderr warning teaches the safe pattern (`burnerpad reveal` + prompt).
- Non-interactive phrase sources, in precedence: `--passphrase-file` > `--passphrase-fd` >
  `BURNERPAD_PASSPHRASE` (documented as weakest) > `--ask` /dev/tty prompt.
- Defense in depth: `output.scrub()` redacts `#…` patterns at the single output boundary;
  `secret.Buffer`'s `Stringer` prints `[redacted]`; release QA greps a recording proxy's full
  traffic for the known key bytes (mechanical enforcement of the SPEC rule).
- Asymmetry: the **mgmt token may appear in argv** (`burn --token`) — it is a destroy-only
  capability; leaking it enables denial, never disclosure.

## Consequences

- Completion scripts never complete positional ids/URLs (no round-trip through completion caches).
- Error messages reference the id only, never the full URL.
