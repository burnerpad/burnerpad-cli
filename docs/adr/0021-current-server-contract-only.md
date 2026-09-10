# ADR-0021: Support only the current burnerpad-lite contract

Date: 2026-09-08 · Status: Accepted

## Context

The unreleased CLI was implemented against burnerpad-lite's retired pre-1.0 API. The current service uses a
different claim method and path, different revoke path and unavailable status, a larger identifier, and
stricter unknown-outcome semantics. Supporting both generations would preserve the removed destructive GET,
require ambiguous capability probing without an API-negotiation endpoint, and multiply the security-sensitive
failure model before the CLI has shipped its first release.

## Decision

The CLI supports only the current, unversioned burnerpad-lite contract. It will not probe for, fall back to,
or retain compatibility paths for retired endpoints or behaviors. The existing implementation and documents
are evidence about prior design work, not constraints on the replacement contract.

Compatibility is established by validating the required security-relevant fields in each operation response
while tolerating additive unknown fields. The CLI does not make a runtime `/api/stats` request or any other
capability probe before an operation; response validation and pinned integration tests provide the contract
check without adding a network race before a destructive request.

Create accepts an optional positive lifetime that resolves to a whole number of seconds. Zero, negative,
fractional-second, and integer-overflow values fail before the request. The CLI sends the requested seconds
without embedding a deployment-specific ceiling, then treats the server's returned `ttl` as the effective
lifetime. It displays that value and explicitly notes when it differs from the request.

## Consequences

- The HTTP layer, tests, terminology, user messages, and documentation are replaced where they encode the
  retired contract.
- No migration behavior is required because there are no released CLI versions or live secrets that depend
  on the old client behavior.
- Future compatibility is measured against burnerpad-lite itself rather than a hand-maintained model of the
  retired server.
- A server may add response metadata without breaking the CLI, but cannot omit or corrupt the fields needed
  to safely complete the operation.
- Server policy, including its default and maximum lifetime, remains owned by the selected deployment.
