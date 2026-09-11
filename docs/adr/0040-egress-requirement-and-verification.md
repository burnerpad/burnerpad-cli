# ADR-0040: Keep the egress requirement distinct from its executable proof

Date: 2026-09-11 · Status: Accepted · Amends: [ADR-0008](0008-no-config-no-state-no-telemetry.md) (verification requirement) · Amended by: [ADR-0042](0042-enforce-process-egress-observation.md) (executable evidence) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md#http-contract), [RELEASING.md](../../RELEASING.md#release-qa-checklist)

## Context

ADR-0008 correctly makes the absence of telemetry, update checks, and compatibility probes part of the
product's privacy boundary. It also says an integration proxy asserts zero egress beyond the API host. The
repository currently contains no such executable proxy assertion, and its real interoperability workflow
does not establish that claim. Labeling the requirement CI-enforced would confuse a structural design review
with observed network evidence.

## Decision

The runtime requirement remains: Burnerpad itself originates no HTTP request to an origin other than the
operation's selected API origin. An operator-selected proxy and operating-system DNS, trust, and networking
facilities are transport dependencies, not additional application-selected destinations. The CLI continues
to have no telemetry, crash reporting, update request, runtime statistics request, or compatibility probe.

This requirement is not currently proven by CI. Until an executable gate exists, release review must describe
the evidence as manual and structural rather than mark it CI-enforced. A sufficient automated gate must run
the real CLI integration matrix inside a deny-by-default network or observing-proxy boundary, account for the
selected API destination and any deliberately configured transport intermediary, fail on every unexpected
application destination, and be a required release dependency. Only then may release documentation claim
that CI proves the no-other-host property.

## Consequences

- The privacy property remains a release requirement; only the strength of the current evidence is
  corrected.
- Unit tests that count API requests or inspect configured URLs remain useful but are not substitutes for
  process-level egress observation.
- A future egress harness must exercise the shipped networking path and be wired into the protected release
  workflow before the checklist can restore a CI-enforced marker.
- Release approval must explicitly assess this manual gate until that work lands.

Retaining the existing CI claim was rejected because the named mechanism does not exist. Dropping the egress
requirement was rejected because unexpected network destinations would violate the no-telemetry and
operation-authoritative-origin boundary even if the intended API request still succeeded.
