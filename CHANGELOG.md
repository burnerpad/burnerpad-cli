# Changelog

All notable changes are documented here. Versions follow Semantic Versioning; the executable version is
derived from the immutable release tag and never hardcoded in source.

## [Unreleased]

## [1.0.0] - 2026-09-10

### Added

- First public command-line release targeting the current burnerpad-lite product contract.
- Browser-compatible suite-`0x02` create, reveal, revoke, and offline recovery workflows.
- Seven-word cryptographically random phrase generation and one canonical 7-64-word shared-list parser.
- Protected passphrase and management-token input through controlling-terminal prompts, named files, and
  descriptors 3 or greater; no secret-bearing argv or environment interfaces.
- Visible burnerpad.io default with configurable self-hosted origins and operation-authoritative URL origins.
- Optional whole-second TTL requests with returned effective-TTL and clamp reporting.
- Owner-only, exclusive plaintext output and claimed-ciphertext recovery files.
- Alternate-screen plaintext viewer, exact piped UTF-8 output, fixed JSON output, and OSC 52 clipboard
  delivery with timed best-effort clearing.
- Frozen exit meanings for input, local I/O, unavailable secrets, passphrase/plaintext failures, definitive
  server rejection, temporary service failures, invalid responses, uncertain mutations, and internal faults.
- One-attempt mutation transport with explicit create/claim/revoke outcome-unknown reporting.
- Pull-request and release interoperability gates against a reviewed burnerpad-lite revision, plus a scheduled
  compatibility run against burnerpad-lite `main`.
- Tag-derived release identity and a release-rendered checksum-pinned installer, with no version constant or
  post-release version-bump commit.

### Security

- Remote origins require verified HTTPS; plaintext HTTP is loopback-only; redirects are refused.
- Every network operation identifies its server before sending and machine responses record that origin.
- Errors exclude links, IDs, phrases, management tokens, ciphertext, plaintext, raw responses, and paths.
- The server API client performs no retry, telemetry, update check, or compatibility probe.
- GitHub Actions and release container images are pinned by immutable digest; unverified package channels are
  excluded from the first release rather than failing or publishing through an unowned destination.

[Unreleased]: https://github.com/burnerpad/burnerpad-cli/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/burnerpad/burnerpad-cli/releases/tag/v1.0.0
