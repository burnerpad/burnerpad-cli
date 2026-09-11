# ADR-0038: Ship version one only from GitHub Releases

Date: 2026-09-11 · Status: Accepted · Supersedes: [ADR-0019](0019-distribution-and-typosquat-defense.md) · Supersedes in part: [ADR-0017](0017-reproducible-builds-keyless-signing.md) (distribution scope) · Amends: [ADR-0008](0008-no-config-no-state-no-telemetry.md) (update discovery) · Source: [CURRENT_SERVER_ALIGNMENT.md](../CURRENT_SERVER_ALIGNMENT.md) §6L, §7

## Context

The original pre-release distribution plan promised package repositories, registry publication, a container
image, and early registration of unrelated package namespaces. This repository records and configures no
owned, end-to-end verified publication and installation path for those channels. Adding them to the first
release would require more credentials and mutable third-party state without improving the trust chain
already implemented by the canonical public repository.

The release pipeline is configured to create a draft in GitHub Releases, attach all verified artifacts, and
publish only after its release-specific gates pass. The committed `install.sh` remains a refusing template;
the publisher is configured to render a version- and checksum-pinned copy for the release.

## Decision

GitHub Releases in `burnerpad/burnerpad-cli` is the sole official binary distribution channel for version
one. Its asset set will contain the six platform archives, configured `deb`/`rpm`/`apk` files, archive
SBOMs, `SHA256SUMS`, the keyless Cosign bundle for that checksum manifest, and the release-rendered
Linux/macOS installer. Windows installation will use a verified release archive; there is no Windows
installer.

Source checkouts remain available for evaluation and audit. Once a version is published, its public Git tag
will remain available as source provenance. A locally built or `go install` binary is not an official
release binary: it is outside the signed asset chain and does not carry the publisher's release metadata.

Version one will not publish to Homebrew, Scoop or another bucket, Winget, AUR, GHCR or another container
registry, or placeholder npm, PyPI, or crates.io packages. Namespace reservation is not a release
prerequisite. A future channel may be advertised only after Burnerpad controls its namespace and has verified
publication, signature or checksum linkage, installation, upgrade, and rollback end to end.

Users will learn about version-one updates through the GitHub Releases feed. External package managers are
not a version-one update channel, amending ADR-0008's broader pre-release consequence.

ADR-0017's reproducible-build, keyless-signing, no-minisign, and deferred-notarization decisions remain in
force. ADR-0034 continues to define release authorization and immutable publication.

## Consequences

- Users will have one canonical binary source and one documented verification chain.
- The publisher needs no package-repository, registry, or placeholder-package credentials.
- The `deb`, `rpm`, and `apk` files will be downloadable GitHub Release assets, not evidence of an apt,
  yum, apk, or other package repository.
- Typosquat defense is an explicit canonical-source warning, not an unsupported claim that Burnerpad controls
  every plausible namespace.
- Adding a distribution channel is a separately reviewed product and release-security change.

## Alternatives considered

- Publish every channel named by ADR-0019 in version one. Rejected because the repository records no owned,
  verified path for those destinations.
- Reserve placeholder names without publishing supported software. Rejected because it creates maintenance
  and impersonation obligations without a verified user path.
- Remove native package files from the GitHub release. Rejected because they will be immutable, checksummed
  assets inside the same release trust boundary and require no external channel.
