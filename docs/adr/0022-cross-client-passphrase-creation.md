# ADR-0022: Official clients create cross-client passphrase secrets

Date: 2026-09-08 · Status: Accepted · Supersedes: [ADR-0006](0006-suite-selection-by-tty.md)

## Context

The unreleased Go CLI selected suite `0x01` when stdout was piped, producing fragment-bearing links that the
current browser deliberately refuses. Burnerpad's browser and Go CLI are two interfaces to the same product:
a secret created by either must be revealable by either, and CLI-to-CLI use must follow the same contract.

## Decision

Every secret created by an official client uses passphrase suite `0x02`, a keyless `/s/<id>` share link, and
a separate passphrase. Output destination and TTY detection do not change the cryptographic suite. The Go CLI
does not create or open suite `0x01`; there are no released CLI versions or live secrets requiring it. The
language-neutral envelope specification may continue to define `0x01` for other consumers, but that does not
expand this product's supported surface. [ADR-0032](0032-one-canonical-passphrase-language.md) defines the one
passphrase language accepted across all official-client operations.

## Consequences

- Go CLI creations can be revealed in the current browser, and browser creations can be revealed by the Go
  CLI.
- Machine-mode creation must carry both the link and passphrase explicitly instead of collapsing the whole
  capability into a fragment-bearing URL.
- The old `--link` creation mode, fragment parsing, link-key inputs, and suite-by-stdout behavior are outside
  the new public contract.
