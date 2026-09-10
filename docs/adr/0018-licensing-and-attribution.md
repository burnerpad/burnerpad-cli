# ADR-0018: Apache-2.0 for the CLI; CC BY 3.0 wordlist attribution embedded in the binary

Date: 2026-08-18 · Status: Accepted · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §13, §26

## Context

The parent chose AGPL for the *server* (protecting the service's openness) and Apache-2.0 for the
*crypto library* (the reusable client surface). The CLI embeds the EFF Short Wordlist #2
(CC BY 3.0 — attribution required), and its `envelope/` package is the "SDK in another language" the
SPEC anticipates.

## Decision

- **Apache-2.0** for this repo: lets `envelope/` be lifted into other projects (growing the auditor
  pool), carries an explicit patent grant (matters for crypto code in a way MIT's silence does not),
  and is compatible in every direction (vendors the Apache spec/vectors; the AGPL server never links
  the CLI). Contributions under the DCO, no CLA — mirroring the parent.
- **Attribution that survives every distribution path**: NOTICE at repo root and in every archive;
  `/usr/share/doc/burnerpad/NOTICE` in deb/rpm/apk; a `go:embed`-ed copy printed by
  `burnerpad licenses` (the only form that survives someone `scp`-ing just the executable); a
  one-line attribution in `burnerpad version`; `burnerpad words` prints one stderr attribution line
  before redistributing the complete licensed work (stdout stays exactly 1296 lines).

## Consequences

- The wordlist is data with an attribution obligation, satisfiable under any code license.
- License hygiene is a release-QA line item (NOTICE presence in archive/packages/`licenses` output).
