# Historical prototypes

The pre-implementation prototypes that lived here (see
[`../docs/design-review-2026-08-18.md`](../docs/design-review-2026-08-18.md)) have all been
used to inform the original pre-release implementation and deleted; their full sources remain in git history
(last present at the tree of the commit that removed them — `git log -- prototype/`). The current product
boundary was redesigned before its first release, so these are historical lineage notes rather than a map of
active packages or gates.

| Tree | Promoted to | Notes |
|---|---|---|
| `core/` | `envelope/` + `wordlist/` | Informed the crypto and wordlist packages; the current suite-`0x02` boundary and tests supersede the prototype. |
| `autocomplete/` | `internal/term/` + `wordlist/` | Informed the phrase editor; the current bounded canonical-phrase machine supersedes the prototype. |
| `diff/` | Retired | The legacy Go↔JS differential harness was removed. Current compatibility gates exercise browser↔CLI and CLI↔CLI flows against a real pinned burnerpad-lite server. |
