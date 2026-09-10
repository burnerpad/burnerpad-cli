# ADR-0013: Every CLI-minted phrase is ≥ 7 uniformly random words — no removal path

Date: 2026-08-18 · Status: Superseded by [ADR-0032](0032-one-canonical-passphrase-language.md) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §6, §13

This record exposed configurable generated word counts and treated supplied phrases byte-verbatim. ADR-0032
matches the browser's seven-word generator and canonical phrase representation for version one.

## Context

Suite 0x02's security ceiling is the phrase's entropy. The web client permits removing generated
words (dropping the random core below 7) and then warns "weaker" — a UI branch that exists only to
be warned about.

## Decision

The CLI makes the weak state unrepresentable: generation is rejection-sampled (no modulo bias),
7 **distinct** words by default (`-w N` raises it, 7 ≤ N ≤ 16); `[a]` *adds* a distinct list word on
top; there is **no removal of generated words** — want different words, reroll. Every CLI-minted
phrase is ≥ ~72.4 bits. Caller-supplied phrases (`--passphrase-file`/`-fd`/env) are used byte-
verbatim — the CLI must not alter bytes it didn't generate, and their strength is the caller's.
(ADR-0020 keeps that byte-verbatim handling but adds a minting-side *refusal*: a supplied phrase the
recipient could not type into the §7.2 prompt is rejected rather than rewritten. Opening is
unchanged.)

## Consequences

- The "weaker phrase" warning UI does not exist in the CLI; there is nothing to test there.
- Stricter than the web on minting while remaining perfectly compatible on opening (free-form mode +
  verbatim file sources open anything conformant).
- No standalone phrase-generator subcommand: it would invite off-label password-manager use;
  `burnerpad words` + standard tools exist for auditors.
