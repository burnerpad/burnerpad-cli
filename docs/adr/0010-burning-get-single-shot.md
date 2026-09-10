# ADR-0010: The burning GET is single-shot; failures are classified, never papered over

Date: 2026-08-18 · Status: Superseded by [ADR-0023](0023-single-attempt-network-operations.md) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §2 (inv. 4), §7.1, §15, §16

This record describes the removed destructive GET and the unreleased retry model built around it. The
current server exposes a destructive JSON POST and official clients do not retry network operations
automatically.


## Context

`GET /api/secrets/:id` destroys the server row before the first response byte. A lost response means
the secret is gone and nobody has it — no retry can exist. Meanwhile Go's `net/http` silently
auto-retries a GET when a *reused* idle connection dies, and redirects/H2 multiplexing muddy the
"did the request reach the server" analysis.

## Decision

- **Everything fallible happens before the fetch** (parse, normalize, pre-validate a fragment,
  collect the complete passphrase) — the ordering invariant of §7.1.
- The take request is issued **at most once per invocation** and never auto-retried after it may
  have reached the server. Structurally enforced: `DisableKeepAlives: true` (a fresh connection can
  never satisfy the stdlib's `isReused()` retry precondition), HTTP/1.1 only (empty `TLSNextProto`),
  redirects never followed.
- Every failure maps into the W0–W6 window table: provably-unsent (retry ≤ 2) vs ambiguous (exit 9
  `take_ambiguous`, fixed golden text) vs burned-and-lost (exit 9 `take_lost`) vs definitive
  outcomes. A definitive 429/503 is proof of no burn (the abuse plug halts pre-dispatch) and is
  retry-safe with `Retry-After` honored (cap 60 s, TTY countdown).
- create/burn/report retry ≤ 2 even on ambiguity (worst case: one orphaned TTL-bounded ciphertext /
  a 403 reported as probable success).

## Consequences

- The exactly-once hazard is surfaced in fixed, golden-tested language — the CLI's contribution is
  refusing to pretend a lost response is recoverable.
- HTTP client config is boring and auditable; keep-alive stays off on the take path forever.
- Intermediary statuses (reverse proxy / CDN 5xx after dispatch) need explicit window
  classification — tracked as a design-review amendment (see docs/design-review).
