# ADR-0039: Freeze two direct Go-team modules and exact public-package boundaries

Date: 2026-09-11 · Status: Accepted · Amends: [ADR-0001](0001-go-as-implementation-language.md), [ADR-0002](0002-one-dependency-freeze.md), [ADR-0004](0004-extractable-stdlib-only-packages.md) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md#components), [`scripts/check-deps.sh`](../../scripts/check-deps.sh)

## Context

The original dependency records described `golang.org/x/term` as the sole direct external module and
`golang.org/x/sys` as transitive. The implementation now deliberately imports both. `x/term` supplies the
terminal primitives, while `x/sys` supplies platform APIs used directly for Unix memory and file hardening,
Windows handles and DACL validation, and the Linux PTY test. Treating a directly used security and platform
dependency as merely transitive makes the frozen graph and its review boundary inaccurate.

The public `envelope` and `wordlist` packages remain intended for extraction without CLI internals or
third-party dependencies. At the network boundary, however, `internal/api` must serialize outbound blobs and
strictly decode inbound blobs and management tokens using `envelope`'s canonical base64url implementation.
The earlier statement that `internal/api` imports `envelope` only for error types does not describe that
necessary use.

## Decision

The module has exactly two direct external requirements: `golang.org/x/term` and `golang.org/x/sys`. Both
are Go-team-maintained, pinned to explicit versions in `go.mod`, and pinned to content hashes in `go.sum`
under the Go checksum-database verification model. No additional production or test-only module is accepted
without a new dependency decision.

`envelope` and `wordlist` remain public root packages that import only the standard library and never import
`internal/`. `internal/api` may import `envelope` to call `EncodeToBytes` and `DecodeCanonical` at the HTTP
boundary. That permission does not move transport, I/O, policy, or logging into either public package.

`scripts/check-deps.sh` enforces only the following mechanical checks:

- `go list -m all`, excluding the main module, must contain exactly the two module paths above;
- the dependency lists for `./envelope` and `./wordlist` must contain neither a `golang.org/x/` package nor
  a package below this module's `internal/` tree; and
- production base64 decoding found by its source scan must remain in `envelope/b64url.go` (test files are
  excluded by that scan).

The script does not prove the complete internal package graph, which internal package may import a public
package, or the purpose for which either direct module is used. Those are architectural review boundaries,
not claims made by that gate.

## Consequences

- The declared module graph matches the packages compiled by the supported platform implementations and
  tests; `go mod tidy` cannot silently demote a deliberately direct requirement.
- Both third-party modules remain visible as first-class supply-chain inputs even where a particular target
  compiles only one of them.
- The gate mechanically rejects the external and `internal/` dependencies that would violate the current
  two-module extraction boundary; review still guards against an unintended root-package dependency.
- Canonical network encoding and decoding have one implementation without pretending that `internal/api`
  is independent of the envelope package.

Keeping `x/sys` nominally transitive was rejected because first-party code imports it directly. Copying its
syscalls and platform structures into the repository was rejected because it would enlarge the most
platform-sensitive code while obscuring its maintained source.
