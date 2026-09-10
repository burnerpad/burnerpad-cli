# ADR-0020: A supplied phrase must be one the recipient can type — validated on minting, never on opening

Date: 2026-08-19 · Status: Superseded by [ADR-0032](0032-one-canonical-passphrase-language.md) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §6.1, §7.2, §7.5

This record rejected non-canonical supplied phrases during creation but accepted arbitrary phrases during
opening. ADR-0032 replaces that asymmetry now that the product boundary is current official clients only.

## Context

Two parties who already share a passphrase want to use it: the sender mints under it, the recipient
opens with it. Before this record the CLI had only `--passphrase-file`/`--passphrase-fd`, both
byte-verbatim and unchecked, and `create` had no way to *type* a phrase at all.

Verbatim is right on the opening side and wrong on the minting side, and ADR-0013 already said so
without enforcing it ("stricter than the web on minting while remaining perfectly compatible on
opening"). The asymmetry has teeth here because of §7.2: the recipient opens with list-locked
autocomplete, where an off-list rune is rejected at the keystroke. A phrase minted from anything
other than list words is not *unopenable* — it is *untypeable there*, and the recipient must find
Ctrl+O free-form entry: masked, no ghost text, no unique-3-prefix guarantee, mistyping possible
again. The sender never sees that cost, which is exactly why the tool must not let them spend it
silently.

## Decision

- `create` gains **`--ask`**: type a pre-shared phrase on `/dev/tty` into the §7.2 prompt with
  `PhraseOpts.NoFreeform` set. Ctrl+O (and the plain-mode `!freeform` line) are refused and never
  advertised, so the prompt's output is mintable by construction. The phrase reaches neither argv,
  nor a file, nor the environment.
- Every caller-supplied phrase on the **minting** path — `--ask`, `--passphrase-file`,
  `--passphrase-fd` — is gated by `wordlist.Validate`: all tokens on the frozen list, single-space
  separated with none at either end, distinct, ≥ `PhraseWords`. That is precisely "reproducible by
  the §7.2 prompt", and a corpus test pins it equivalent to `term.SeedWords(p, 0) != nil`, the §7.3
  retry seeder — the same rule's other implementation — so the two cannot drift.
- **No escape hatch.** This CLI does not mint a phrase the recipient cannot type. A caller who wants
  a high-entropy non-list phrase has `-L` link mode, whose key is uniformly random and needs no
  typing at all.
- **Opening is never validated.** `reveal` and `decrypt` keep every §7.5 source byte-verbatim and
  keep free-form entry, so phrases minted by other clients — which the envelope format permits to be
  arbitrary UTF-8 — open normally.
- Refusal is exit 2, pre-network, nothing created. The message names the **position** and the reason
  and never the token: it lands on stderr, and in a script stderr is one CI-log line from disclosing
  key material — the same reasoning that gates a piped phrase in §5.2. `wordlist.PhraseError` carries
  no phrase bytes at all, so the property is structural rather than a discipline at each call site.
- `--ask` conflicts with `--passphrase-file`/`--passphrase-fd` (exit 2). Reveal's §7.5 precedence
  lets `--ask` override a file source; minting cannot borrow that rule, because silently ignoring a
  supplied phrase would mint under a *different* phrase than the caller asked for.

## Consequences

- ADR-0009 is untouched: the supply channels are a terminal, a file, and an fd. There is still no
  `--passphrase <words>` in the grammar, and this record does not create a reason for one — `--ask`
  is the ergonomic answer to "I already have the phrase" that argv appeared to offer.
- ADR-0013's "caller-supplied phrases are used byte-verbatim" now holds only for the opening path.
  Minting keeps byte-verbatim *handling* — no lowercasing, no whitespace collapsing, nothing is
  altered — and adds refusal. Validation rejects; it never rewrites.
- **This is not an entropy claim, and must not be read as one.** The rule makes a supplied phrase
  typable, not strong: seven list words a human chose are seven words a human chose. The ~72.4-bit
  figure of §13 describes the generator, and applies only to phrases this CLI generated.
- A pre-existing script supplying a non-list phrase to `create` now fails loudly at exit 2 instead of
  minting a secret with a degraded open experience. Deliberate: the failure is cheap, pre-network,
  and names its fix, while the state it replaces was invisible to the person who caused it.

## Alternatives rejected

- **Warn and mint anyway.** A sender in a pipe with `--quiet` never sees it, which is exactly the
  population that mints unopenable-in-practice secrets in bulk.
- **An `--any-phrase` escape hatch.** Every caller who hits the error would reach for it, which
  converts a guarantee into a default-on lint. Link mode already covers the legitimate case.
- **Validating on the opening path too.** It would make the CLI unable to open conformant secrets
  minted by other clients — a format-compatibility break dressed as a UX improvement.
