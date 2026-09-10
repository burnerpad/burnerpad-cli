# ADR-0026: Official CLI secrets are well-formed UTF-8 text

Date: 2026-09-10 · Status: Accepted

## Context

The browser creates and displays text. A Go-only binary creation or output mode would produce secrets that
cannot satisfy the required browser-to-CLI and CLI-to-browser interoperability, while the product is intended
for passwords, tokens, configuration blocks, and similar textual material.

## Decision

The CLI accepts, creates, reveals, and locally decrypts only well-formed UTF-8 plaintext. Create input must be
non-empty and is limited to 65,491 encoded bytes, matching the browser's suite-`0x02` plaintext budget under
the server's 64 KiB blob limit. It validates create constraints before any network request and performs no
Unicode normalization. Binary creation and binary output modes are not part of the public command surface.

## Consequences

- Every secret created by an official client can be represented by the other official client.
- Authenticated but non-UTF-8 plaintext is rejected rather than partially displayed or rewritten.
- The internal cryptographic primitive may continue to operate on bytes, but that does not expand the CLI's
  product contract.
