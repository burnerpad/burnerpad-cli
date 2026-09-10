# ADR-0012: List-locked autocomplete as a pure state machine — ghost text, no auto-commit

Date: 2026-08-18 · Status: Superseded by [ADR-0032](0032-one-canonical-passphrase-language.md) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §7.2, §7.6

The interaction design remains useful, but its free-form escape and foreign-client compatibility boundary do
not belong to the current official-client-only product. ADR-0032 retains autocomplete around one canonical
passphrase language.

## Context

Typing a 7-word phrase blind invites typos that surface only as a silent `auth_fail` after a burned
fetch. The wordlist's unique-3-prefix property makes each word determinable in 3 keystrokes. The
web client proves the interaction; the terminal needs an equivalent without a TUI framework
(ADR-0002) that survives conhost/tmux/screen readers.

## Decision

- **Rendering: inline ghost text, no dropdown** — one line, one repaint (CR + EL on a single
  physical row; head elision `…` so wrap can never occur; SIGWINCH → one repaint).
- **No auto-commit on unique prefix**: a unique prefix triggers *display* (the ghost shows the whole
  word); commitment is an explicit Space/Tab/Enter — the defense against valid-prefix-of-the-wrong-word
  slips (`tup` → tupperware when tulip was meant, §10c). **Tab accepts** the ghosted word (commit +
  clear buffer, the browser gesture) rather than only extending to the longest common prefix: on a
  unique-at-3 list the LCP branch is near-dead, and a Tab that completed a word but left the caret
  inside it made the user type a Space the completion had already implied. The slip defense is
  untouched — Tab is as deliberate a keystroke as Space, and the ghost has shown the whole word first.
- Keystrokes that would leave zero candidates are rejected (bell + status) — a malformed phrase is
  unrepresentable; canonical form (lowercase, single spaces) is produced by construction.
- Bracketed paste is atomic: all tokens valid → all commit; any invalid → whole paste rejected.
- **Ctrl+O free-form mode** (masked line entry) covers non-wordlist phrases other clients may mint —
  list-locking is a burnerpad-phrase optimization, not a format rule.
- **Architecture: a pure state machine** (key event in → committed/buf/ghost/bell/status out) with
  the renderer owning width — fully unit-testable without a PTY. `--plain` replaces it with
  line-based validated entry as a first-class accessibility mode, not a degraded afterthought.

## Consequences

- ~250–400 first-party lines + an exhaustive table-driven test matrix; one Linux PTY smoke test
  drives the real binary end-to-end.
- Phrase entry echoes words (threat model is history/logs, not shoulder-surfing); free-form mode is
  masked since foreign phrases may be password-like.
