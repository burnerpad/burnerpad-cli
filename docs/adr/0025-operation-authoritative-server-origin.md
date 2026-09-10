# ADR-0025: Each network operation has one authoritative server origin

Date: 2026-09-10 · Status: Accepted

## Context

The CLI defaults to burnerpad.io while supporting self-hosted instances. A reveal link already identifies the
server holding its ciphertext, whereas create has no link yet and a bare-ID revoke carries no origin. Allowing
a global server option to silently redirect a share link would make the selected service difficult to audit.

## Decision

Every network operation exposes its resolved server origin. Human mode displays it before the request even
under quiet mode; machine success and error objects include it as `server`. Create resolves `--server`, then
`BURNERPAD_SERVER`, then `https://burnerpad.io`. Reveal accepts a full share URL whose origin is authoritative;
it does not accept a bare ID, and an explicitly supplied `--server` is ignored with a warning. Passing the
share link in argv remains supported but warns that this destructive capability is now in shell history.

Revoke uses the server recorded in a piped create receipt, otherwise a full target URL's origin, otherwise
`--server`, `BURNERPAD_SERVER`, or the burnerpad.io default for a bare ID. If an explicit server option is
irrelevant because the receipt or URL already identifies the server, the CLI warns rather than switching
silently.

## Consequences

- A reveal can never claim the same identifier from a different server because of ambient configuration.
- Self-hosted reveal links work without a separate server flag.
- Automation receives the selected origin in structured output; human output writes it to stderr without
  printing a secret identifier or request path.
