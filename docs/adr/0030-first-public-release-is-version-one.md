# ADR-0030: Make the aligned CLI's first public release version one

Date: 2026-09-10 · Status: Accepted

## Context

The repository contains an extensive but unreleased interface designed for a retired server contract. One
option was to publish the replacement as `v0.1.0` and allow the public surface to soak. The project instead
chooses to finish the current-server alignment before publication and make an explicit compatibility promise
from its first release.

## Decision

The first public release of the aligned CLI is `v1.0.0`. The current command surface, JSON schemas, exit
statuses, pipe formats, supported crypto suite, and documented security behavior are reviewed and tested as
the version-one contract before that tag is created. No compatibility is owed to behavior that existed only
in the repository before `v1.0.0`.

## Consequences

- The implementation may remove all obsolete pre-release behavior during alignment.
- Once `v1.0.0` is published, incompatible public-interface changes follow semantic versioning and require a
  new major release.
- The release gate must exercise the actual browser/CLI interoperability and current burnerpad-lite contract,
  because there is no post-release compatibility trial period.
