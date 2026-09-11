# Changelog

All notable changes are documented here. Versions follow Semantic Versioning; the executable version is
derived from the immutable release tag and never hardcoded in source.

## Unreleased

### Added

- First public command-line release targeting the current burnerpad-lite product contract.
- Browser-compatible suite-`0x02` create, reveal, revoke, and offline recovery workflows.
- Seven-word cryptographically random phrase generation and one canonical 7-64-word shared-list grammar
  across interactive, file, and descriptor input.
- Protected passphrase and management-token input through controlling-terminal prompts, named files, and
  descriptors 3 or greater; no secret-bearing argv or environment interfaces.
- Visible burnerpad.io default with configurable self-hosted origins and operation-authoritative URL origins.
- Optional whole-second TTL requests with returned effective-TTL and clamp reporting.
- Owner-only, exclusive plaintext output and claimed-ciphertext recovery files.
- Bounded, terminal-sized alternate-screen plaintext paging, exact piped UTF-8 output, and fixed JSON output.
- Frozen exit meanings for input, local I/O, unavailable secrets, passphrase/plaintext failures, definitive
  server rejection, temporary service failures, invalid responses, uncertain mutations, and internal faults.
- One-attempt mutation transport with explicit create/claim/revoke outcome-unknown reporting.
- Pull-request and release interoperability gates against a reviewed burnerpad-lite revision, plus a scheduled
  compatibility run against burnerpad-lite `main`.
- Tag-derived release identity and a release-rendered checksum-pinned installer, with no version constant or
  post-release version-bump commit.
- Truthful installation guidance for the release-rendered installer and pre-release source evaluation.

### Changed

- Accept only zero installer arguments or the exact `--verify-only` form; typos and extra arguments now fail
  with usage before platform detection, download, or installation.
- Describe interactive phrase acceptance as submission in every command instead of incorrectly promising a
  decrypt operation during `create --ask`.
- Generate Bash, Zsh, Fish, and PowerShell command/option vocabularies from the runtime flag schema; leading
  options no longer hide the command, value positions no longer suggest flags, and offline decrypt no longer
  suggests network flags.
- Generate general help and all nine command topics from the command schema and scoped flag sets;
  `COMMAND --help` and `COMMAND -h` now match `help COMMAND` without executing the target command.

### Removed

- Remove the inert `--quiet` option from parsing, help, completions, and documentation; supplying it is now an
  invalid option instead of silently producing unchanged output.
- Delete the unused retained-word retry state, seed parser, rendering branch, and duplicate tests; local
  wrong-phrase retries still re-enter a complete phrase against the already-held ciphertext without a network
  retry.
- Stop accepting inert `burn --plain` and `burn --no-color`; burn has no phrase editor, viewer, or colored
  output for those terminal-presentation controls to affect.

### Security

- Remote origins require verified HTTPS; plaintext HTTP is loopback-only; redirects are refused.
- Every network operation identifies its server before sending and machine responses record that origin.
- Errors exclude links, IDs, phrases, management tokens, ciphertext, plaintext, raw responses, and paths.
- The server API client performs no retry, telemetry, update check, or compatibility probe.
- GitHub Actions and release container images are pinned by immutable digest; unverified package channels are
  excluded from the first release rather than failing or publishing through an unowned destination.
- Render decrypted plaintext as inert visible escapes in interactive terminals without changing pipe/file
  bytes or decoded JSON values.
- Acquire the implicit viewer's controlling terminal before a destructive reveal request can consume the secret.
- Classify every undocumented post-send mutation status as operation-specific outcome unknown.
- Cancel and join commands on the first termination signal so transmitted mutations remain classifiable,
  create/reveal handoffs are not abandoned, pending post-claim terminal delivery remains persistently visible,
  and viewer Ctrl+C exits as an interruption.
- Retire OSC 52 clipboard options before version one because terminal acceptance and payload completeness
  cannot be verified, keeping exit zero's destination-delivery contract unqualified.
- Enforce the raw upstream conformance-vector hash, execute every supported and generic encoding vector, and
  make byte-for-byte specification drift a pull-request and release dependency.
- Bound bracketed paste and interactive lines at their allocation boundary, wiping and draining oversized
  input before a later prompt can consume it.
- Keep rejected typed, pasted, and plain-line passphrase material out of interactive diagnostics.
- Enforce current-user ownership, regular-file type, owner-only modes without macOS extended ACLs or
  owner-only Windows DACLs, and final-component
  symlink/reparse refusal for named passphrase and management-token files before network access.
- Warn before credential input or network access when `burn` receives a full share URL in argv; bare IDs and
  piped create receipts remain silent because they do not place a share URL in shell history.
