# ADR-0017: Reproducible builds + Sigstore keyless signing; no minisign; notarization deferred

Date: 2026-08-18 · Status: Accepted in part · Release authorization superseded by: [ADR-0034](0034-trusted-default-branch-immutable-releases.md) · Distribution scope superseded by: [ADR-0038](0038-github-releases-only-for-version-one.md) · Source: historical pre-release architecture

## Context

A one-maintainer secrets tool's most attractive attack target is its release path. A long-lived
minisign/GPG key on a laptop is a single point of theft; hosted repos and manifests can be hijacked.

## Decision

- **Reproducible builds by construction**: toolchain pinned by `go.mod` `toolchain` directive (repo
  decides the compiler), committed `go.sum` + sumdb, one canonical flags block
  (`CGO_ENABLED=0 -trimpath -buildvcs=false -ldflags "-s -w -buildid=" -X main.*`), commit-derived
  timestamps. A `repro-verify` job rebuilds every release in a *different* environment and compares
  binary hashes; mismatch opens a release incident.
- **Signing: SHA256SUMS + cosign keyless** (OIDC identity = the trusted default-branch publisher).
  Every signature lands in Rekor, and immutable publication adds a GitHub release attestation —
  silent re-release becomes impossible. The `--bundle` embeds the Rekor proof for offline
  verification. **No minisign** (a
  second, weaker root of trust). One signature covers everything (sign SHA256SUMS; artifacts verify
  via `sha256sum -c`).
- goreleaser v2 is the single release driver (archives, SBOMs, nfpm deb/rpm/apk, tap/bucket/winget/
  AUR pushes, `FROM scratch` container). ADR-0034 replaces the original protected-environment trigger
  with a configured-actor default-branch dispatch and immutable draft publication.
- **macOS notarization / Windows Authenticode: skipped at launch** with documented fallbacks
  (recommended channels never set the quarantine xattr; SmartScreen reputation starts at zero even
  with a paid cert). Revisit only if direct-download demand from non-developers materializes.
- **UPX rejected**: top AV/EDR heuristic, breaks Mach-O signatures and repro diffing.

## Consequences

- `go install …/cmd/burnerpad@latest` is the zero-trust auditor path (sumdb-verified, no release
  infra involved).
- Release QA checklist (§27) makes each CI-enforced invariant a human-confirmed line item.
