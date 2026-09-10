# ADR-0029: Use secure standard HTTP transport with one bounded attempt

Date: 2026-09-10 · Status: Accepted · Supersedes: [ADR-0014](0014-http-client-posture.md)

## Context

The old transport policy forced HTTP/1.1, disabled connection reuse, and offered an insecure-HTTP escape
hatch because it was modeled around a retired destructive GET. The current server uses POST mutations and
requires the client to report uncertain outcomes honestly rather than trying to eliminate them through
transport quirks.

## Decision

Remote server origins must use HTTPS. Plain HTTP is accepted only for loopback origins; the old
`--insecure-http` option is removed. TLS uses the platform trust store and cannot be configured to skip
certificate verification. Redirects are never followed.

The CLI honors the conventional proxy environment and otherwise uses Go's normal HTTP/1.1 or HTTP/2
negotiation. Each operation makes one network attempt. Its default total timeout is 12 seconds and may be
configured by the caller; a timeout or incomplete response after a mutation is classified according to the
operation's outcome-unknown rules, never automatically replayed.

## Consequences

- Self-hosted local development remains simple without weakening remote transport.
- Standard protocol negotiation and proxy behavior work without expanding the confidentiality controls.
- Callers may choose a different wait bound, but not automatic retry or insecure TLS behavior.
