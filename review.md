# Burnerpad CLI deep review

Date: 2026-09-10
Original reviewed snapshot: `e8edfba6e345be9a314b2c298763f7cdb2d50086`
Canonical public-repository baseline: `1219efe5f356077cef33c770c00ac9cd022af1a5`
Status: open; work through findings in priority order

## Verdict

This is not yet the smallest possible implementation, it does not fully satisfy its own specification, and it should not be tagged `v1.0.0` yet.

| Dimension | Verdict |
|---|---|
| Cryptographic core | Strong |
| Network boundary | Mostly strong |
| Release security | Hardened implementation; canonical repository activated |
| Destructive-operation safety | Several high-risk gaps |
| Minimality | Dependency-minimal, not code-minimal |
| Completeness | Happy paths exist; documented guarantees are incomplete |
| Intuitiveness | Good automation interface; uneven interactive UX |

## Remediation tracker

- [x] 1. Prevent release-protection bypass and tag-triggered shell execution.
- [x] 2. Render decrypted plaintext safely in interactive terminals.
- [x] 3. Preflight the implicit viewer before destructive reveal.
- [x] 4. Make signals mutation-aware and run required cleanup.
- [x] 5. Resolve the unverifiable OSC 52 delivery contract.
- [x] 6. Correct post-mutation HTTP outcome classification.
- [x] 7. Enforce the advertised conformance-vector pin.
- [x] 8. Repair deterministic release reproduction.
- [ ] 9. Fix remaining completeness and UX defects.
- [ ] 10. Remove dead and duplicated surface area.

## Critical and high findings

### 1. Critical — the protected release environment can be bypassed

Workflow-level `contents: write`, `id-token: write`, and `attestations: write` apply to the unprotected gate jobs before the protected environment is reached (`.github/workflows/release.yml:9-24`, `:80-96`).

This is compounded by shell injection in `Makefile:4-17`: `VERSION` comes from `git describe` and is interpolated inside double-quoted shell source. Git accepts tags containing quotes and semicolons; a safe dry-run with a crafted `v*` tag produced a command that escaped `go build` and executed a second shell command.

Unless tag creation is separately protected by repository settings—which cannot be verified from this tree—someone able to push a `v*` tag can run code with repository-write and OIDC authority before manual approval, publish artifacts, and obtain a signing identity matching `SECURITY.md:44-52`.

Required changes:

- In `.github/workflows/release.yml`, default workflow permissions to read-only or none.
- Grant `contents: write`, `id-token: write`, and `attestations: write` only to the protected `release` job.
- Set `persist-credentials: false` on non-release checkouts.
- Strictly validate an immutable semver tag before any build step.
- Stop interpolating untrusted version metadata into shell source.
- In GitHub repository settings, protect immutable `v*`/semver tags and restrict who can create them.

The first five items are tracked repository code/config changes. The final item is an external GitHub repository-settings change.

Remediation applied 2026-09-10:

- Removed the tag-triggered publisher and replaced it with
  `.github/workflows/publish-release.yml`, which GitHub loads only from the
  default branch through `repository_dispatch`.
- Restricted the dispatch to the numeric account ID in the
  `RELEASE_ACTOR_ID` repository variable, pins the request to `GITHUB_SHA`,
  validates canonical SemVer before checkout, serializes releases, and grants
  write/OIDC authority only to the final publishing job.
- The publisher creates or verifies the exact tag, builds a replaceable draft
  using exact tool versions, verifies the required assets and keyless checksum
  bundle, and confirms `isImmutable` after publication. The setting is an
  operator prerequisite because GitHub requires admin access to read it and
  the publisher intentionally has only contents/OIDC authority.
- Build metadata is frozen, allowlisted, passed as data rather than generated
  shell source, and covered by adversarial legal-Git-tag and Make expansion
  tests.
- The original private repository's historical workflow was disabled before
  the project was re-imported into the canonical public
  `burnerpad/burnerpad-cli` repository. Repository settings do not transfer
  with `.git`: the canonical repository was therefore audited and activated
  separately. Its history contains no legacy tag-triggered publisher; default
  workflow permissions are read-only; immutable releases are enabled; Actions
  must use full commit SHAs; the default branch and `v*` tags are protected;
  and `RELEASE_ACTOR_ID=1019893` is authorized only after those controls are
  verified. The public repository could add another build-attestation action,
  but intentionally keeps the smaller Cosign-checksum plus immutable-release
  attestation design.

### 2. High — decrypted plaintext can execute terminal control sequences

Both terminal modes emit sender-controlled plaintext essentially verbatim (`internal/term/viewer.go:43-55`, `:71-75`). Valid UTF-8 permits ASCII ESC and other controls.

A malicious sender can embed OSC 52 clipboard writes, terminal-title changes, deceptive hyperlinks, screen clearing, or an alternate-screen exit sequence. This can also defeat the “nothing enters scrollback” claim.

Interactive display should visibly escape control characters. Exact bytes can remain available through explicit `--out`, JSON, or piped stdout.

Remediation applied 2026-09-10: both alternate-screen and plain/fallback
terminal modes now share a streaming safe renderer. Graphic UTF-8 and logical
line breaks remain readable; C0/C1 controls, DEL, Unicode format characters,
and malformed bytes become inert Go-style escapes. The renderer never creates
a second full plaintext buffer, and the header retains the original byte
count. Piped stdout and files retain the original authenticated bytes, and decoded
JSON round-trips them. The OSC 52 destination was subsequently retired in Finding 5.
Regression tests cover OSC 52 and OSC 8 attacker strings, alternate-screen
exit injection, control/format characters, CR/LF normalization, graphic
Unicode, and malformed UTF-8 in both viewer modes.

### 3. High — reveal can consume the secret before discovering that its viewer is unavailable

Explicit output and clipboard destinations are prepared at `internal/cli/reveal.go:76-90`, but the destructive claim happens at `:98`. The implicit viewer does not acquire its controlling terminal until `:208-214`.

With TTY stdout but no controlling terminal—for example under `setsid` or some containers—the secret is consumed, viewer acquisition fails, and no recovery ciphertext exists unless `--keep-blob` was selected. This contradicts the destination-preflight guarantee.

Remediation applied 2026-09-10: destination preparation now opens and caches
the controlling terminal whenever implicit TTY viewing is selected. Delivery
reuses that exact prepared terminal from the application cache, so a missing
controlling terminal fails locally before the reveal POST in both
alternate and plain modes. Explicit files, JSON, and piped stdout
keep their existing destination-specific preflight behavior. Regression tests
prove terminal-open failure sends zero claim requests and that JSON, file, and
piped-stdout destinations do not acquire a viewer terminal.

### 4. High — signal handling can hide a completed mutation and lose data

`Run` launches dispatch in a goroutine and immediately returns on SIGINT/SIGTERM (`internal/cli/run.go:97-114`). `main` then calls `os.Exit`, while network operations use `context.Background()`.

A signal arriving after transmission can therefore:

- create a secret without returning its link or token;
- consume a one-time secret before it is saved or displayed;
- revoke a secret while reporting only exit 130/143.

This should be an operation-specific outcome-unknown result once request transmission may have started.

The same lifecycle causes two related bugs:

- SIGINT during the clipboard countdown exits before the clear sequence at `internal/cli/reveal.go:198-207`.
- Ctrl+C inside the raw viewer returns success because `internal/term/viewer.go:78-87` returns `nil`.

Remediation applied 2026-09-11:

- One Run-owned context now covers input, all three mutation requests, terminal waits, and timed cleanup. The
  first SIGINT/SIGTERM cancels and joins dispatch; an already-transmitted mutation that cannot be confirmed
  retains its operation-specific exit-`9` result, while definitive completed results remain authoritative.
- A queued signal before dispatch sends no request, simultaneous completed dispatch wins deterministically,
  and production signal notification stops after the first signal so a second regains immediate OS behavior.
- Terminal input now uses cancellation-aware, TTY-owned reads. Password/raw/alternate-screen/bracketed-paste
  state is synchronously restored; viewer Ctrl+C is an interruption; non-EOF viewer errors remain local errors.
- Confirmed one-time plaintext that becomes ready after cancellation is not flashed and erased: terminal
  handoff becomes persistent plain output. The clipboard cleanup path implemented with this lifecycle change
  was subsequently removed along with the unverifiable destination in Finding 5.
- Regression coverage exercises pre-dispatch SIGINT/SIGTERM, create/claim/revoke post-send ambiguity, confirmed
  and failed handoffs, active pipe cancellation, retry-error preservation, real Linux PTY restoration, Windows
  compilation, and deterministic signal/result precedence.

### 5. High — OSC 52 success is reported despite delivery being unverifiable

The implementation admits that terminal clipboard success cannot be verified (`internal/term/osc52.go:11-17`). Nevertheless, reveal says plaintext was copied and returns zero, whose documented meaning is “artifact reached its destination” (`README.md:158-168`).

Unsupported or size-limited terminals may silently discard or truncate the sequence after the one-shot secret has been consumed. For destructive reveal, require recovery preservation or weaken the success contract and message honestly.

Remediation applied 2026-09-11:

- Removed `--clip` from create, reveal, and decrypt and deleted the OSC 52, tmux-passthrough, countdown,
  and clearing implementation. No native clipboard helper replaced it.
- The built-in destinations are now the safe terminal viewer, exact non-terminal stdout, exclusive owner-only
  files, and JSON. External clipboard pipelines are caller-owned; destructive reveal recommends `--keep-blob`
  when an external handoff may need recovery.
- Retired flag forms fail with exit 2 during parsing, before a create or claim request. Regression tests cover
  the destructive reveal boundary and prove help and all four runtime completion scripts omit the option.
- Removed the four duplicate checked-in completion artifacts; the runtime `completion` command is now their
  single source. ADR-0035 records the pre-version-one contract reversal and its amendments to ADR-0028 and
  ADR-0033. No GitHub repository setting is required for this change.
- Verified with the full Go test and vet suites, Staticcheck, the race suite, six cross-builds, the binary-size
  gate, diff checks, and independent standards/spec reviews.

### 6. High — mutation status classification reports false certainty

After every POST, `internal/api/api.go:224-243` treats most unexpected sub-500 responses as protocol errors rather than outcome unknown. Examples include `201`, `204`, `302`, and operation-inapplicable `400`/`413` statuses.

The mutation may already have occurred, but exit 8 says only “invalid server response”; the documented contract reserves outcome unknown for incomplete or invalid post-send mutation results (`docs/CURRENT_SERVER_ALIGNMENT.md:287-291`). The existing redirect test pins the incorrect classification.

Remediation applied 2026-09-10: mutation status classification is now
operation-specific and conservative. Only create `400`/`413`, claim/revoke
`404`, and the shared `429`/`503` statuses establish documented failures.
Every other final status after transmission returns the operation's outcome-
unknown error; `200` still requires a complete valid operation-specific body.
An exhaustive status matrix covers HTTP 100–599 for create, claim, and revoke,
and the redirect regression now proves one request with an unknown claim
outcome rather than a protocol-only failure.

## Release and conformance gaps

### 7. The advertised vector pin is not enforced

`envelope/vectors.go:3-8` says the harness independently verifies `VectorsSHA256`, but `envelope/suite02_vectors_test.go:42-61` only unmarshals the file. It requires each class to be nonempty, so most vectors could disappear while tests still pass. The generic `encoding` and `encoding_negative` arrays are not loaded at all.

The current file does match the declared hash; the problem is that the release gate does not prove it. Spec drift runs only after main pushes or on schedule, not on PRs or as a release dependency (`.github/workflows/spec-drift.yml:5-8`).

Remediation applied 2026-09-11:

- The harness hashes the raw `testdata/v1.json` bytes before decoding and requires the digest advertised by
  `VectorsSHA256`. Fixture fields distinguish absence from valid empty values, and new expectation strings
  fail closed.
- Every applicable suite-`0x02` decrypt, encrypt, and negative vector now runs, along with every generic
  encoding and encoding-negative vector in both relevant directions. The raw hash is the single exact-set pin,
  so the harness does not duplicate brittle fixed counts.
- `spec-drift` now runs on pull requests and as a reusable workflow. Both checkouts discard credentials, the
  reviewed Lite pin must be a full lowercase commit SHA and match the checkout, and a missing vendored
  specification can no longer be silently skipped.
- Release drift runs only after request authorization, and the privileged publisher directly depends on it.
  The active `Protect default branch` ruleset now requires the registered `drift` check; no user-side GitHub
  action, secret, token, or environment remains outstanding.
- Verified with the pinned upstream byte comparison, full Go test/vet/race suites, Staticcheck, actionlint,
  shell syntax, six cross-builds, the size gate, the live required PR check, and three independent final
  reviews.

### 8. Reproducibility verification is guaranteed to disagree with GoReleaser

GoReleaser embeds `main.date={{ .CommitDate }}` at `.goreleaser.yaml:12-16`, while `.github/workflows/repro-verify.yml:42-50` rebuilds using `YYYY-MM-DD`. GoReleaser defines `.CommitDate` as UTC RFC3339 in its [official template documentation](https://www.goreleaser.com/customization/general/templates/), so the binaries necessarily differ. Commit representations likely differ too.

The verifier also interpolates tag inputs into shell source, and the privileged release job downloads floating `~> v2` GoReleaser rather than an exact version.

Remediation applied 2026-09-10: the publisher pins GoReleaser `v2.18.1` and
Syft `v1.51.1`, and all release tag inputs pass the shared canonical SemVer
policy through environment variables. GoReleaser and the clean-container
rebuild now both embed the full commit SHA and the UTC RFC3339 commit date.
Private release assets are fetched through the authenticated GitHub API, and
the publisher explicitly dispatches the independent reproduction run. The
generated installer now validates and embeds the actual `GITHUB_REPOSITORY`
instead of assuming a future repository owner.

## Completeness and UX defects

The application implements the core create/reveal/burn/decrypt paths, but not everything it documents:

- Errors during local wrong-phrase retry are converted to exit 10 instead of preserving Ctrl+C/local errors (`internal/cli/reveal.go:111-125`, `internal/cli/output.go:110-117`).
- The viewer has no paging or scrolling; a supported 65,491-byte secret leaves only its tail visible.
- Interactive phrase entry accepts more than 64 words and Unicode whitespace, while file/fd parsing caps at 64 and permits ASCII whitespace only (`internal/term/machine.go:455-484`, `wordlist/validate.go:9-40`).
- Bracketed paste and plain prompt lines are unbounded in memory.
- Create’s phrase prompt says “Enter decrypts.”
- `--quiet` is documented and parsed but never consulted.
- The seeded retry subsystem is substantial but never used; users retype the complete phrase.
- Bash, Zsh, and PowerShell suggest unsupported `decrypt --timeout`; Bash and Zsh break when a global option precedes the command.
- `burn` accepts a share URL in argv without the promised history warning.
- README has no installation instructions; command help is mostly a single usage line, only four help topics exist, and `create --help` fails.
- The installer silently ignores unknown or extra arguments, so a mistyped `--verify-only` can install.
- Rejected pasted tokens are quoted in errors despite the secret-free error guarantee.
- Credential files called “protected” are opened without checking ownership, type, permissions, or symlinks.
- `scripts/sync-vectors.sh` uses a predictable `/tmp/bp-wordlist.$$` path.
- The changelog says 1.0.0 is released, while this checkout has no tag, the installer is still a refusing development template, and release tasks remain unchecked.
- `burnerpad words` omits the stderr attribution required by accepted ADR-0018 before redistributing the
  embedded CC BY 3.0 wordlist.

Finding 9 task ledger:

- [x] 9.1 Preserve interruption and local-I/O errors during wrong-phrase retry.
- [x] 9.2 Add bounded paging/scrolling to the interactive viewer.
- [x] 9.3 Make interactive phrase parsing use the canonical 7–64-word ASCII-whitespace grammar.
- [x] 9.4 Bound bracketed-paste and plain prompt-line memory.
- [x] 9.5 Replace create's misleading “Enter decrypts” prompt copy.
- [x] 9.6 Remove the inert `--quiet` surface.
- [x] 9.7 Remove the unused seeded-retry subsystem.
- [x] 9.8 Correct completion grammar and unsupported flag suggestions.
- [x] 9.9 Warn when `burn` receives a share URL in argv.
- [x] 9.10 Add installation guidance and complete command help behavior/topics.
- [x] 9.11 Make the installer reject unknown and extra arguments.
- [x] 9.12 Keep rejected pasted tokens out of diagnostics.
- [x] 9.13 Enforce the protected credential-file ownership/type/mode/symlink contract.
- [ ] 9.14 Replace the predictable vector-sync temporary path.
- [ ] 9.15 Make all pre-release changelog, installer, documentation, and task claims truthful.
- [ ] 9.16 Restore the documented wordlist attribution on stderr.

## Minimality assessment

There are 4,670 production Go lines and 4,159 test lines. `internal/term` alone is 1,862 production lines—about 40% of the executable—and has 2,181 test lines.

A conservative cleanup can remove or consolidate roughly 600-800 code/test/config lines without removing a current requirement:

- Dead spinner: exactly 111 implementation/test lines.
- Unused seeded-retry machinery: roughly 400 implementation/test lines.
- Remove inert `--quiet`.
- Generate completions from one command schema instead of keeping embedded and standalone copies.
- Share reveal/decrypt authentication, retry, UTF-8 validation, and delivery logic.
- Remove test-only exported interfaces.
- Archive approximately 800 lines of completed/stale planning documents from the active documentation surface.

Using a deep-module/deletion-test lens, `envelope`, `id`, `api`, and `secret` are mostly good, deep modules. `internal/term` is the outlier: large surface, many historical/test-only interfaces, and several defects leaking across CLI behavior.

Finding 10 task ledger:

- [ ] 10.1 Delete the unused spinner implementation and tests.
- [x] 10.2 Delete the unused seeded-retry subsystem (completed with 9.7).
- [x] 10.3 Delete the inert `--quiet` configuration and interface (completed with 9.6).
- [x] 10.4 Replace duplicated completion vocabularies with one command schema (completed with 9.8).
- [ ] 10.5 Share reveal/decrypt authentication, retry, validation, and delivery flow.
- [ ] 10.6 Remove test-only exported terminal interfaces and wrappers.
- [ ] 10.7 Remove stale completed planning documents and the obsolete prototype.

## What is strong

- AES-256-GCM, fresh salt/IV, PBKDF2-SHA256 at 600,000 rounds, and authenticated envelope metadata are implemented cleanly.
- Canonical base64url parsing is strict and fail-closed.
- Current suite vectors, RFC vectors, and pinned Wycheproof data pass.
- URL/origin parsing is defensive: HTTPS remotely, HTTP only on loopback, no credentials/path/query/fragment, no redirects, normal platform trust roots.
- Response sizes are bounded and mutation requests are never automatically retried.
- Output/recovery file creation is exclusive and owner-only, including explicit Windows DACL handling.
- There is no application subprocess execution, telemetry, updater, config state, or compatibility probe.
- Runtime dependencies are only `x/term` and `x/sys`.
- No credible repository secret or known dependency vulnerability was found.

## Verification and scope

All 152 tracked files—20,710 lines total—were accounted for:

- 72 Go files;
- 48 Markdown files, including every ADR;
- all seven workflows;
- every script, completion, build/release file, integration test, and manpage;
- the vendored specification, wordlist, suite vectors, and both Wycheproof datasets.

Large generated or vendored datasets were fully hashed, parsed, schema/conformance-tested, and scanned rather than interpreted as prose line by line. The six ignored `dist/` binaries were metadata-inspected separately; they are stale development artifacts (`7c5ffcc-dirty`) rather than current HEAD (`e8edfba`) and are not tracked.

Checks completed successfully:

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `gofmt`
- Staticcheck
- `govulncheck`: no vulnerabilities
- dependency/import/decode gate
- size gate
- shell and JavaScript syntax checks
- pinned dataset hashes
- full-history secret scan: no credible secret
- aggregate Go coverage: 68.5%

The original audit made no application changes; remediation began afterward
and is recorded inline above.

## Remediation verification

The completed release-path changes were checked with:

- `go test ./...`, `go vet ./...`, and `go test -race ./...` on Go 1.27.1;
- the focused adversarial release/Make/installer regression suite;
- actionlint 1.7.12 on both changed workflows;
- GoReleaser 2.18.1 `check`;
- all six cross-builds and the size gate; and
- an actual GoReleaser snapshot whose binary reports the full reviewed commit
  and `2026-09-10T16:26:52Z`, exactly matching the independent rebuild inputs.

Safe terminal rendering was checked with focused adversarial viewer tests in
both terminal modes plus the full local Go test, race, vet, fuzz, and
cross-build suites.

After the move, the canonical public repository was re-read independently:
default workflow permissions are read-only, immutable releases are enabled,
all Actions references are full commit SHAs, the default branch and release
tags are protected by active rulesets, and `RELEASE_ACTOR_ID` is `1019893`.
Its history contains no legacy release workflow. No release or tag was created
during remediation.

## Recommended order

1. Release permissions and tag handling.
2. Safe terminal rendering and pre-claim destination validation.
3. Signal and mutation lifecycle.
4. HTTP outcome classification.
5. Conformance and reproducibility gates.
6. Simplification and UX cleanup.
