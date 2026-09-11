# ADR-0031: Require explicit commands and protected input sources

Date: 2026-09-10 · Status: Accepted · Supersedes: [ADR-0009](0009-key-material-never-in-argv.md) · Amended by: [ADR-0037](0037-validate-named-credential-files.md)

## Context

Claiming and revoking are destructive. The unreleased CLI accumulated shorthand commands, implicit reveal,
and ambient credential sources that saved typing but made intent and provenance harder to audit. Version one
has no compatibility obligation to that surface.

## Decision

The only command names are `create`, `reveal`, `burn`, `decrypt`, `words`, `completion`, `version`, `licenses`,
and `help`; there are no command aliases or implicit `burnerpad <URL>` reveal. A claim always requires the
explicit `reveal` verb. A share URL supplied in argv remains supported with the agreed shell-history warning.

Create takes non-empty UTF-8 plaintext from the interactive composer, piped stdin, or `--input FILE`, with
exactly one source. Plaintext is never accepted from argv or the environment. Passphrases use the terminal,
`--passphrase-file`, or `--passphrase-fd`, with exactly one source and no argv, environment, or stdin form.

Burn takes its management token from a piped create receipt, `--token-file`, `--token-fd`, or a protected
terminal prompt. It accepts no token value in argv or the environment and no mixed credential sources. The
explicit command plus valid management token is sufficient intent; there is no additional confirmation.

## Consequences

- Shell history and process listings never receive plaintext, passphrases, or management tokens from the CLI
  interface.
- Stdin has one unambiguous primary artifact per invocation.
- Scripts written against pre-release aliases or ambient credentials are deliberately unsupported.
