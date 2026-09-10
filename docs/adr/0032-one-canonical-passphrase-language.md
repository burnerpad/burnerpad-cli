# ADR-0032: Use one canonical passphrase language across official clients

Date: 2026-09-10 · Status: Accepted · Supersedes: [ADR-0012](0012-list-locked-autocomplete-design.md), [ADR-0013](0013-minted-phrases-floor.md), [ADR-0020](0020-supplied-phrases-must-be-mintable.md)

## Context

The old CLI supported adjustable generated phrases and arbitrary byte-verbatim phrases when opening. Those
paths served foreign and retired clients, while the current product requires browser-to-CLI, CLI-to-browser,
and CLI-to-CLI compatibility only. Multiple phrase dialects would make pre-claim validation weaker and the
version-one interface larger.

## Decision

The generator always produces exactly seven distinct uniformly selected words from the shared EFF wordlist;
there is no `--words` option. Caller-supplied and entered phrases contain 7–64 distinct shared-list words.
Every create, reveal, and local-decrypt input uses the same rule.

Input may contain leading, trailing, or separating ASCII whitespace for file and paste ergonomics. Before
encryption or key derivation, the CLI parses the words and represents the phrase canonically as lowercase
words joined by single ASCII spaces. It never changes a word or its order. Off-list words, duplicates, and
out-of-range word counts are rejected without echoing any supplied token. Interactive entry retains
list-locked autocomplete and atomic paste, but has no free-form escape.

## Consequences

- Formatting differences such as a file's final newline cannot create an accidental incompatible key.
- Every accepted phrase can be entered in the current browser, and obvious phrase-shape mistakes fail before
  a destructive claim.
- Supporting arbitrary suite-`0x02` passphrases in the future would be an explicit expansion of the product
  boundary rather than retained pre-release behavior.
