# ADR-0027: Claimed ciphertext recovery is explicit and never logged

Date: 2026-09-10 · Status: Accepted · Supersedes: [ADR-0011](0011-blob-preservation-and-decrypt.md)

## Context

After a successful claim, the client may hold the last ciphertext copy. Local phrase correction is valuable,
but automatically printing that ciphertext into stderr or JSON turns logs and CI artifacts into unintended
storage and conflicts with the current official-client logging policy.

## Decision

Wrong-phrase attempts retry locally against the held blob. A caller may select `--keep-blob FILE` before the
claim; the CLI validates that exclusive destination before sending the request, then writes the blob
immediately after a successful claim and before decryption. The file is mode `0600`, contains only the
canonical unpadded-base64url blob followed by one newline, and remains until the user removes it.

The local `decrypt --blob-file FILE` command accepts only that format, with `-` selecting stdin. Legacy raw
binary and JSON recovery formats are rejected. Ciphertext is never included in ordinary stderr or JSON
errors. Exiting without an explicit recovery destination warns that the held ciphertext will be abandoned.

## Consequences

- Recovery remains possible without treating output streams as ciphertext storage.
- Destination failures occur before the irreversible claim wherever they can be detected.
- The user must explicitly choose durable recovery; the CLI does not create recovery files by default.
- An explicitly requested recovery file remains even after successful plaintext delivery; cleanup is an
  explicit user action rather than an implicit data-loss decision.
