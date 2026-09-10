# ADR-0014: HTTP/1.1 only, keep-alives off, no redirects, and no TLS-verify-skip flag — ever

Date: 2026-08-18 · Status: Superseded by [ADR-0029](0029-current-transport-policy.md) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §15

This record coupled HTTP/1.1-only connections and a non-loopback insecure-HTTP escape hatch to the retired
burning-GET analysis. ADR-0029 replaces that unreleased policy for the current POST-only contract.

## Context

The client makes 1–2 tiny sequential requests, one of which is irreversibly destructive. Every
protocol convenience (H2 multiplexing, connection reuse, redirect following, `--insecure`) either
muddies the exactly-once failure analysis or is a foot-gun for a confidentiality tool.

## Decision

Stdlib `net/http` with: HTTP/1.1 only (empty `TLSNextProto`), `DisableKeepAlives: true`
(load-bearing — see ADR-0010), redirects never followed (a redirect on an API path is a
misconfigured or hostile server; following one could replay the burning GET elsewhere → exit 8),
TLS ≥ 1.2 with system roots (Linux/BSD honor `SSL_CERT_FILE`/`SSL_CERT_DIR`), body reads capped at
200 KB, User-Agent `burnerpad-cli/<version>` and nothing else. **There is no flag to skip TLS
certificate verification, ever** — an active MITM on the take path steals a secret that can never be
re-fetched. Dev/self-host paths: loopback plain HTTP needs no flag; non-loopback `http://` requires
the deliberately scary `--insecure-http`; private CAs go into the OS trust store.

## Consequences

- Strict response parsing (`DisallowUnknownFields` + presence/type checks) freezes the server's tiny
  response schemas as wire contract for this client; a server that grows them ships a CLI release
  first. Fail closed cuts both ways.
- Proxies honored via `ProxyFromEnvironment`; requests never compressed; response gzip left on
  (payloads are ciphertext — no compression-oracle surface).
