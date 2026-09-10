# ADR-0019: Distribution channels and the typosquat defense

Date: 2026-08-18 · Status: Accepted · Amended by: [ADR-0030](0030-first-public-release-is-version-one.md), [ADR-0031](0031-explicit-commands-and-protected-inputs.md) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §4.1, §24, §27

## Context

The binary is named `burnerpad` (`bp` and `burner` rejected: namespace squatting / collisions).
Users install security tools from wherever search lands them; unclaimed namespaces become
impersonation surface precisely when the project gets notable.

## Decision

- **Root channel**: GitHub Releases (archives + SHA256SUMS + cosign bundle + SBOM); everything else
  derives from it. At v1.0.0 this includes archives, nfpm deb/rpm/apk artifacts, the `go install`
  path, and the tag-rendered installer. A channel is not advertised or updated until its org-owned
  repository or package namespace exists and the release path has been verified end to end.
- **Register early, everywhere plausible** — tap, bucket, winget publisher, AUR names, ghcr
  namespace at v1.0.0; genuine minimal placeholder packages on npm/PyPI/crates.io where policy
  permits. The README lists the complete official-channel set: "if it isn't on this list, it isn't
  us."
- **`curl | sh` demoted and hardened**: `install.sh` is in-repo, documented as "download, read, then
  run", embeds per-target SHA256s (trust root = the script you read), never uses sudo, installs to
  `~/.local/bin`, fails closed, offers `--verify-only`.
- Channel repos are org-owned, 2FA-enforced, branch-protected, updated only by the release
  workflow's scoped token; manifests point at cosign-covered artifacts so a swap is detectable.

## Consequences

- Shipped completions mitigate the long name; `alias bp=burnerpad` remains a user-owned shell choice rather
  than a second CLI command surface.
- Releases never gate on community downstreams (aports/ports/Debian).
