# ADR-0042: Enforce per-origin process egress observation in real-client CI

Date: 2026-09-11 · Status: Accepted · Amends: [ADR-0008](0008-no-config-no-state-no-telemetry.md) (executable evidence), [ADR-0040](0040-egress-requirement-and-verification.md) (automated proof) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md#release-and-compatibility-gates), [RELEASING.md](../../RELEASING.md#release-qa-checklist)

## Context

ADR-0040 requires process-level evidence that the real CLI contacts no origin other than the one selected for
its operation. Request-counting unit tests and source review cannot observe a hidden telemetry, update, or
compatibility request. A passive packet capture can record traffic without making a missing observation fail
closed, while a single allowlist containing every server used anywhere in the suite cannot prove the
per-operation origin boundary.

The real-client suite also runs Node, Chromium, request fixtures, and CLI children together. Network evidence
from that process tree cannot honestly attribute every packet to the CLI alone; the functional assertions and
the process boundary must be interpreted together.

## Decision

Use one Linux-only `scripts/run-lite-interop.sh` harness in pinned pull-request CI, scheduled Lite-main
compatibility CI, and the release gate. All dependency installation, image pulling, and both Lite boots finish
before observation starts. The pinned Playwright image is then run with `--pull=never`, host networking and
the host user namespace, fixed unprivileged UID 60000, all capabilities dropped, no-new-privileges, empty
proxy variables, and read-only source and binary mounts.

Before assigning that UID, the harness rejects host account, process-credential, or Internet-socket ownership
collisions. First-position IPv4 and IPv6 `OUTPUT` owner jumps lead to unique deny-by-default chains. Numeric
loopback canaries must increment both reject counters before the evidence counters are zeroed. The six normal
tests then run with only TCP `127.0.0.1:4014` returned to the existing host policy; the expiry test runs
separately with only TCP `127.0.0.1:4015` returned. Each phase must both exercise its selected-port rule and
leave its IPv4 and IPv6 reject counters at zero. `RETURN`, rather than `ACCEPT`, preserves every pre-existing
host `OUTPUT` rule.

The observed unit is the whole Playwright container process tree: Node, Chromium, request fixtures, and every
CLI child. The functional tests establish which real CLI operation selected each origin; the firewall proves
that nothing in that exercised process tree used a different IP destination. The evidence does not cover
pre-gate setup, the host Lite processes, the Docker daemon, kernel-generated packets without a socket owner,
non-IP IPC, dormant code paths, or macOS and Windows executables. Those boundaries remain covered by the
small import graph, structural tests, cross-platform tests and builds, and review; the automated egress claim
is deliberately limited to the exercised Linux real-client paths.

Cleanup stops the exact named containers before deleting owner jumps and their unique chains, then stops both
Lite processes and deletes its private temporary files. It removes the firewall boundary only after positively
confirming that every client container is stopped; if Docker cannot establish that postcondition, the UID
rules remain on the disposable runner. Mutation intent is recorded before firewall changes so a signal cannot
strand a rule between a successful command and later bookkeeping. A cleanup failure converts an otherwise
successful run to failure.

## Consequences

- Unexpected IPv4 or IPv6 traffic from an exercised client process is rejected and makes all three real-client
  workflows fail.
- Normal and expiry operations cannot use each other's permitted origin without incrementing a reject counter.
- Positive selected-port counters plus functional CLI assertions provide evidence without pretending packet
  attribution is finer than the process boundary.
- The release gate now supplies the executable evidence ADR-0040 required; scheduled Lite-main uses the same
  implementation but remains separate compatibility evidence.

A union allowlist for ports 4014 and 4015 was rejected because it weakens the invariant from selected origin to
either test origin. An observing proxy was rejected because raw sockets, DNS, and proxy bypass would remain
outside its view. Replacing the host firewall policy with unconditional accepts was rejected because a test
must not weaken its runner's existing network controls.
