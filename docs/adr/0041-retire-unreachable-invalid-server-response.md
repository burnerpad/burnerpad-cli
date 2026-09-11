# ADR-0041: Retire the unreachable invalid-server-response error

Date: 2026-09-11 · Status: Accepted · Amends: [ADR-0028](0028-version-one-machine-interface.md) · Source: [README.md](../../README.md#output-contract), [ARCHITECTURE.md](../ARCHITECTURE.md#machine-and-exit-contract)

## Context

The unreleased machine interface included `invalid_server_response` at exit `8` and an API `ProtocolError`
type, but no production HTTP path constructed that type. The only way to emit the code was a synthetic unit
test. Once a mutation has been transmitted, a malformed or undocumented final response cannot establish that
the operation did not happen; the current API therefore classifies that result as the operation-specific
exit-`9` outcome-unknown error. Advertising a code that real execution could not emit enlarged the frozen
interface without giving callers a useful distinction.

## Decision

Remove `invalid_server_response` and `ProtocolError` before the first public release. Exit `8` remains part
of the version-one exit-status interface, but its sole JSON code is `unsupported_secret`: local ciphertext
that is not canonical Burnerpad base64url or is not a supported passphrase envelope. Malformed successful
mutation responses and undocumented final mutation statuses continue to produce the operation-specific
exit-`9` outcome-unknown code; transport, rate-limit, and temporary service failures remain exit `7`.

The production registry is the closed JSON-error vocabulary. An exhaustive process-level test must exercise
every registered code, reject missing or extra table cases in both directions, and compare the complete JSON
line so field presence and ordering remain deliberate. `retry_after` is emitted after `server` only for a
valid non-negative delta-seconds value supplied by the server; zero is valid, while malformed and negative
values are omitted.

## Consequences

- Exit `8` has one reachable, precise meaning instead of combining local ciphertext compatibility with a
  dead server-response category.
- Automation cannot branch on a promised error that no real invocation can produce.
- A future reachable server-response category requires an explicit interface decision and real-path test;
  it cannot be added by reviving the unused type.

Keeping the ghost code for hypothetical future use was rejected because versioned interfaces should describe
observable behavior. Mapping malformed mutation responses to exit `8` was rejected because it would falsely
claim a destructive operation had a known outcome.
