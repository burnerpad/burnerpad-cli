# Architecture Decision Records

ADRs 0001–0020 were extracted from the unreleased historical
[ARCHITECTURE.md](../ARCHITECTURE.md). ADRs 0021 onward record the current-server redesign, with
[CURRENT_SERVER_ALIGNMENT.md](../CURRENT_SERVER_ALIGNMENT.md) specifying its exact approved behavior. Each
record preserves why a decision stands and marks displaced pre-release decisions as superseded.

| # | Decision | Anchors |
|---|---|---|
| [0001](0001-go-as-implementation-language.md) | Go as the implementation language (floor go1.25) | §3 |
| [0002](0002-one-dependency-freeze.md) | One-dependency freeze; hand-rolled terminal layer | M6, §19 |
| [0003](0003-conformance-as-release-gate.md) | Envelope-spec conformance as merge + release gate (superseded) | M2, §14 |
| [0004](0004-extractable-stdlib-only-packages.md) | Public, extractable, stdlib-only `envelope/` + `wordlist/` | §11, §13, §19 |
| [0005](0005-single-base64-decode-gate.md) | One audited canonical-base64url decode gate | M1, §11.4 |
| [0006](0006-suite-selection-by-tty.md) | Default suite by stdout TTY-ness (superseded) | §5.2 |
| [0007](0007-stream-discipline-and-machine-surface.md) | Stream discipline, `--json`, exit codes as frozen surface (superseded) | §5.1, §8.3, §9 |
| [0008](0008-no-config-no-state-no-telemetry.md) | No config file, no state, no telemetry, no update check | §4.5, §18, §25 |
| [0009](0009-key-material-never-in-argv.md) | Key material never in argv/requests/logs (superseded) | M5, §4.2, §17 |
| [0010](0010-burning-get-single-shot.md) | The burning GET is single-shot; W0–W6 classification (superseded) | §7.1, §15, §16 |
| [0011](0011-blob-preservation-and-decrypt.md) | Blob preservation post-burn; `decrypt` recovery command (superseded) | §7.3, §7.4 |
| [0012](0012-list-locked-autocomplete-design.md) | Autocomplete as pure state machine; ghost text; no auto-commit (superseded) | §7.2, §7.6 |
| [0013](0013-minted-phrases-floor.md) | CLI-minted phrases ≥ 7 generated words, no removal path (superseded) | §6, §13 |
| [0014](0014-http-client-posture.md) | HTTP/1.1 only, keep-alives off, no redirects, no TLS-skip flag (superseded) | §15 |
| [0015](0015-best-effort-memory-hygiene.md) | Best-effort memory hygiene; `[]byte` secrets; no memguard | M4, §12 |
| [0016](0016-clipboard-osc52-only.md) | Clipboard via OSC 52 only; `--clip` on reveal with timed clear (superseded) | §8.1 |
| [0017](0017-reproducible-builds-keyless-signing.md) | Reproducible builds + keyless signing; no minisign | §22–23 |
| [0018](0018-licensing-and-attribution.md) | Apache-2.0; embedded CC BY 3.0 attribution | §13, §26 |
| [0019](0019-distribution-and-typosquat-defense.md) | Distribution channels & typosquat defense | §4.1, §24, §27 |
| [0020](0020-supplied-phrases-must-be-mintable.md) | Supplied phrases validated on minting, never on opening (superseded) | §6.1, §7.2, §7.5 |
| [0021](0021-current-server-contract-only.md) | Support only the current burnerpad-lite contract | Current integration boundary |
| [0022](0022-cross-client-passphrase-creation.md) | Official clients create cross-client passphrase secrets | Current client boundary |
| [0023](0023-single-attempt-network-operations.md) | Network operations are single-attempt and expose unknown outcomes | Current mutation semantics |
| [0024](0024-supported-crypto-conformance.md) | Gate conformance on the supported passphrase suite | Current crypto boundary |
| [0025](0025-operation-authoritative-server-origin.md) | Each network operation has one authoritative server origin | Current origin policy |
| [0026](0026-official-secrets-are-text.md) | Official CLI secrets are well-formed UTF-8 text | Current content boundary |
| [0027](0027-explicit-claimed-blob-recovery.md) | Claimed ciphertext recovery is explicit and never logged | Current recovery policy |
| [0028](0028-version-one-machine-interface.md) | Freeze the minimal JSON and exit-code interface for version one | Current machine interface |
| [0029](0029-current-transport-policy.md) | Use secure standard HTTP transport with one bounded attempt | Current transport policy |
| [0030](0030-first-public-release-is-version-one.md) | Make the aligned CLI's first public release v1.0.0 | Release compatibility boundary |
| [0031](0031-explicit-commands-and-protected-inputs.md) | Keep destructive commands explicit and credentials out of ambient inputs | Version-one command surface |
| [0032](0032-one-canonical-passphrase-language.md) | Use one canonical passphrase language across official clients | Version-one phrase boundary |
| [0033](0033-explicit-plaintext-destinations.md) | Make plaintext destinations explicit and non-overwriting | Version-one output policy |
| [0034](0034-trusted-default-branch-immutable-releases.md) | Release only from the trusted default branch into an immutable draft | Release authorization boundary |
| [0035](0035-retire-unverifiable-clipboard-delivery.md) | Retire unverifiable clipboard delivery before version one | Destination-success boundary |
| [0036](0036-retire-inert-quiet-option.md) | Retire the inert quiet option before version one | Minimal command surface |

New ADRs: next number, same template (Date/Status/Source, Context, Decision, Consequences,
Alternatives where meaningful). Superseding an ADR: new record, link both ways, flip the old
Status to `Superseded by NNNN`.
