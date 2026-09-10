# ADR-0023: Network operations are single-attempt and expose unknown outcomes

Date: 2026-09-08 · Status: Accepted · Supersedes: [ADR-0010](0010-burning-get-single-shot.md)

## Context

Create, claim, and revoke mutate server state before a complete response is guaranteed. A connection failure
or invalid response can therefore leave the client unable to know whether the operation happened. The old
CLI automatically retried several network paths, which could hide that uncertainty or create duplicate,
unreachable ciphertext rows.

## Decision

The CLI makes one network attempt per user-requested operation. It does not automatically resend after
transport failure, wait and resend after rate limiting, or retry because a failure appears provably early.
Any further network attempt is a deliberate new user action. Wrong-passphrase retries remain unlimited only
after a blob is held locally, because those are local retries and cannot claim server state again.

## Consequences

- Failures distinguish definitive rejection or unavailability from an outcome that is unknown.
- `Retry-After` may be reported to the user but never triggers an automatic request.
- The HTTP implementation need not preserve the obsolete burning-GET retry windows.

