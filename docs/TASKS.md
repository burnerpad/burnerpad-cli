# v1 implementation checklist

This checklist tracks the current-server rewrite defined by
[`CURRENT_SERVER_ALIGNMENT.md`](CURRENT_SERVER_ALIGNMENT.md). Historical pre-v1 tasks were removed because
they described a different product.

## Product and crypto

- [x] Support only suite `0x02` in the shipped Go envelope API.
- [x] Canonicalize 7–64 distinct shared-list words identically for create, reveal, and decrypt.
- [x] Generate exactly seven distinct uniformly sampled words.
- [x] Gate every applicable upstream suite-`0x02` vector plus PBKDF2 and Wycheproof primitives.
- [x] Accept only non-empty UTF-8 create plaintext up to 65,491 bytes.

## Current server contract

- [x] Implement `POST /api/secrets` with optional whole-second TTL and required effective TTL response.
- [x] Implement one-attempt `POST /api/secrets/:id/reveal` with `{}`.
- [x] Implement one-attempt `POST /api/secrets/:id/burn` with a protected management token.
- [x] Collapse unavailable reveal/revoke states to the current generic 404.
- [x] Tolerate additive response fields while requiring and validating consumed fields.
- [x] Remove automatic retry, delay, backoff, and runtime compatibility probes.
- [x] Distinguish definite temporary inability from post-send mutation outcome unknown.
- [x] Retain normal Go proxy and HTTP negotiation, system roots, loopback-only HTTP, and no redirects.

## CLI surface

- [x] Exact commands: create, reveal, burn, decrypt, words, completion, version, licenses, help.
- [x] Remove aliases, implicit reveal, report, suite switches, adjustable words, force overwrite,
  insecure HTTP, argv tokens, and passphrase environment input.
- [x] Default to burnerpad.io while visibly disclosing the selected origin.
- [x] Make a reveal URL authoritative and warn when explicit `--server` is irrelevant.
- [x] Support protected passphrase/token file, fd, and controlling-terminal sources.
- [x] Freeze v1 success/error JSON and exit meanings.

## Recovery and output

- [x] Reserve owner-only output and recovery files before claim.
- [x] Never overwrite an existing destination.
- [x] Write opted-in recovery ciphertext immediately after claim and retain it on all later outcomes.
- [x] Keep wrong-phrase retry local to held ciphertext.
- [x] Support a terminal-safe alternate/plain viewer, exact piped UTF-8, exclusive file output, and JSON.
- [x] Keep unverifiable terminal clipboard delivery outside the built-in destination contract.
- [x] Keep secrets, paths, raw responses, and ciphertext out of errors.

## Compatibility and release

- [x] Record the reviewed burnerpad-lite revision in `.burnerpad-lite-revision`.
- [x] Gate PRs and releases on browser→CLI, CLI→browser, and CLI→CLI flows against that revision.
- [x] Run the same Chromium matrix nightly against burnerpad-lite `main`.
- [x] Derive v1 versions from Git tags with no source version bump.
- [ ] Observe one successful scheduled Lite-main workflow run after merge.
- [ ] Dispatch `v1.0.0` only after immutable releases are enabled and all repository-required checks are green.
