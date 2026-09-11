# Contributing

Thanks for helping. This repo is deliberately small and strict; most friction
below exists to protect properties users depend on.

## Ground rules

- **DCO, no CLA.** Sign your commits: `git commit -s` (see [DCO](DCO)).
- **The design is binding.** [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
  specifies every flag, exit code, byte offset, and failure behavior. Behavior
  changes start as a doc change (plus an ADR when a decision is being
  revisited — see [docs/adr/](docs/adr/)).
- **Dependency freeze.** `go.mod` may require exactly `golang.org/x/term` and
  `golang.org/x/sys` — including for tests. CI fails otherwise
  (`scripts/check-deps.sh`). Don't propose adding a dependency without an ADR;
  the answer is usually a few hundred lines of first-party code instead.
- **Conformance is a gate, not a goal.** Every applicable suite-`0x02` case in
  `envelope/testdata/v1.json` must be green on every commit. Never edit vendored files
  (`spec/SPEC.md`, the vectors, the wordlist) except via a reviewed upstream
  re-pin (`scripts/sync-vectors.sh`).
- **Secrets are `[]byte`, never `string`.** No secret material in error
  messages, logs, or argv-defined flags — the review gates grep for it.

## Building and testing

```sh
make build          # canonical flags block
make test           # full suite incl. every applicable suite-0x02 vector
make lint           # gofmt + vet (+ staticcheck when installed)
make fuzz           # bounded local fuzz of the attacker-byte parsers
go test ./...         # process/API contract tests and supported vectors
```

The CI interoperability job checks out the revision in `.burnerpad-lite-revision`, starts the real server,
and drives browser→CLI, CLI→browser, and CLI→CLI flows. Update the pin only with a reviewed compatibility run.

## Manual smoke checklist (Windows / macOS interactive paths)

CI covers the pure state machine and the Linux PTY end-to-end run; before a
release touching `internal/term`, walk this by hand where possible:

1. Windows Terminal: `burnerpad create` full interactive flow; autocomplete
   ghost text renders; alt-screen viewer opens and restores.
2. Legacy conhost: VT-enable fails → automatic `--plain`; plain print carries
   the scrollback warning.
3. Ctrl+C at each phase of `reveal`: pre-claim (exit 130, nothing lost),
   post-claim with `--keep-blob`, and viewer.
4. macOS Terminal + iTerm2: raw-mode prompt, bracketed paste of a full phrase,
   SIGWINCH mid-prompt.

## Pull requests

- One logical change per PR; tests in the same PR.
- User-facing changes update `CHANGELOG.md` (CI checks the `user-facing`
  label) and the man page source when flags/output change.
- Golden-test updates must be justified in the PR description — fixed message
  texts are part of the compatibility promise.
