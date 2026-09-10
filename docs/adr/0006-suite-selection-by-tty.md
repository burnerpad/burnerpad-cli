# ADR-0006: Default suite on `create` selected by stdout TTY-ness

Date: 2026-08-18 · Status: Superseded by [ADR-0022](0022-cross-client-passphrase-creation.md) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §5.2

This record describes the unreleased pre-1.0 design. Official Go CLI creation now follows the browser's
suite `0x02` contract so secrets interoperate in both directions.


## Context

The two suites serve two audiences with different security models. A human wants the two-channel
model (key-less link + spoken phrase — what the web offers). A script wants exactly one composable
string on stdout; forcing 0x02 there pushes phrase and link through the *same pipe*, destroying the
two-channel benefit anyway. The SPEC treats both suites as first-class for any client; the web's
0x02-only stance is a browser-channel policy, not a format rule.

## Decision

**stdout is a TTY → suite 0x02 (passphrase); stdout is piped → suite 0x01 (link).** stdin's TTY-ness
selects only the secret source, never the suite. Guardrails against "silent context-dependent
crypto": the chosen suite is always announced on stderr and carried in `--json`; `-L`/`-P` always
override; the rule is mechanical (stdout TTY-ness is the only input). Piped create with `-P`
requires `--json` or a caller-supplied phrase source — a generated phrase would otherwise have
nowhere safe to go (exit 2).

## Consequences

- `burnerpad create` and `burnerpad create > url.txt` mint different suites — deliberate, visible
  via the announcement, and documented in the transcripts (§10a/b).
- The CLI is a strict superset of the web client's crypto: it both mints and opens 0x01.
- Machine-to-machine handoffs get the honest single-artifact model (the URL is the whole
  capability); humans keep the two-channel model.
