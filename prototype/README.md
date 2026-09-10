# Prototypes — promoted and removed

The pre-implementation prototypes that lived here (see
[`../docs/design-review-2026-08-18.md`](../docs/design-review-2026-08-18.md)) have all been
promoted into the real packages and deleted; their full sources remain in git history
(last present at the tree of the commit that removed them — `git log -- prototype/`).

| Tree | Promoted to | Notes |
|---|---|---|
| `core/` | `envelope/` + `wordlist/` (T1.1) | Production sources and the conformance/fuzz/wordlist test suites carried over verbatim (wordlist tests extended with the B23 prefix-freeness gate). |
| `autocomplete/` | `internal/term/` + `wordlist/` (T5.2) | Full test matrix absorbed and extended (machine, render, §10(c) transcript, typability sweep, yo-yo fix). The one prototype-only test not ported, `TestYoYoUnreachableUnderStrictCharset`, demonstrated the defect of a rejected *strict-charset* design that the shipped machine deliberately does not have. |
| `diff/` | `internal/difftest/` (T1.5) | The Go↔JS differential harness is now a permanent gate: `go test -tags diff ./internal/difftest -run TestDifferential` (weekly + pre-release via `.github/workflows/diff-weekly.yml`; volumes tunable via `DIFF_N` / `DIFF_FAULTS`). |
