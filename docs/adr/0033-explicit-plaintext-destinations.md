# ADR-0033: Make plaintext destinations explicit and non-overwriting

Date: 2026-09-10 · Status: Accepted · Supersedes: [ADR-0016](0016-clipboard-osc52-only.md)

## Context

Revealed plaintext can escape through terminal scrollback, permissive output files, clipboard history, or
machine logs. At the same time, refusing useful output modes merely pushes users toward less controlled shell
pipelines. Version one needs predictable destinations whose risks are visible before a claim occurs.

## Decision

Reveal and decrypt use the alternate-screen viewer when stdout is a terminal and emit exact UTF-8 plaintext
when stdout is not a terminal. `--out FILE` creates an exclusive mode-`0600` plaintext file and never
overwrites; there is no `--force`. JSON mode uses the fixed ADR-0028 schemas. `--json`, `--out`, and `--clip`
are mutually exclusive plaintext destinations.

Clipboard access is opt-in OSC 52 only, including tmux passthrough, with no helper executables.
`create --clip` copies the share link and never the phrase. `reveal --clip[=DURATION]` and
`decrypt --clip[=DURATION]` copy plaintext instead of displaying it, default to 45 seconds, keep the process alive,
and then attempt to clear the clipboard. The CLI warns that clearing is best-effort and clipboard managers
may retain history. Clipboard and output destinations are validated before a network claim.

## Consequences

- Existing files are never destroyed as a side effect of reveal.
- Clipboard behavior is consistent for network reveal and local recovery without executing PATH-resolved
  programs.
- A terminal that cannot support the requested destination fails as a local I/O error before claim whenever
  that fact can be established.
