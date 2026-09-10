# Releasing

Every release, however trivial, goes through the full pipeline — no hand-built
binaries, ever (ARCHITECTURE.md §25). The publisher is loaded only from the
current default branch, pins the requested commit, and re-runs the entire test
suite before it receives publication authority.

## Required GitHub settings

Before attempting a release:

1. Keep the historical tag-triggered `.github/workflows/release.yml` workflow
   permanently disabled. It is removed from the default branch, but disabling
   its GitHub workflow ID is what prevents an old commit from reviving it.
2. Set the repository's default workflow permission to read-only.
3. Enable immutable releases and verify that
   `gh api repos/OWNER/REPOSITORY/immutable-releases --jq .enabled` prints
   `true`.
4. Set `RELEASE_ACTOR_ID` to the numeric ID of the only account authorized to
   dispatch a release:

   ```sh
   gh variable set RELEASE_ACTOR_ID \
     --body "$(gh api user --jq .id)" \
     --repo OWNER/REPOSITORY
   ```

On the current private GitHub Free repository, branch/tag rulesets, protected
environments, and GitHub Actions build attestations are unavailable. Repository
dispatch by the account named in `RELEASE_ACTOR_ID` is therefore the release
authorization boundary; compromise of that account is not mitigated by
repository controls. The numeric ID remains valid if the repository moves to an
organization. If the repository becomes public or moves to a plan with stronger
controls, add reviewed-pull-request protection to the default branch and an
independent approval environment.

The publisher creates the tag itself at the authorized default-branch tip.
GoReleaser builds a draft, all assets and the keyless Cosign checksum bundle are
attached and verified, and the draft is published only at the end. GitHub then
locks the published release, its assets, and its tag and creates the release
attestation.

## Cut a release

1. Ensure `CHANGELOG.md` has the version's entry (Keep-a-Changelog format;
   security entries list affected versions and advisory IDs).
2. Fetch the remote default branch and record its full commit:

   ```sh
   git fetch origin main
   release_sha=$(git rev-parse origin/main)
   ```

3. Request the release, replacing the version and repository:

   ```sh
   gh api --method POST repos/OWNER/REPOSITORY/dispatches \
     -f event_type=release \
     -f "client_payload[tag]=vX.Y.Z" \
     -f "client_payload[sha]=$release_sha"
   ```

4. Confirm `publish-release` created the tag, attached the installer,
   checksums, Cosign bundle, and SBOMs, and published the immutable release. If
   `main` advanced before the publishing job created the tag, the workflow
   intentionally aborts; fetch and request it again.
5. Wait for the explicitly dispatched `repro-verify` run before announcing.
   Do not make a post-release version-bump commit.

The publisher retries that dispatch three times. If publication succeeded but
all three dispatch attempts failed, start the verifier without changing the
release:

```sh
gh workflow run repro-verify.yml \
  -f tag=vX.Y.Z \
  --repo OWNER/REPOSITORY
```

If a run fails after creating its tag or draft, do not move the tag. Confirm it
still resolves to the requested commit before retrying; GoReleaser replaces an
incomplete draft for the same tag. If an immutable release is deleted, GitHub
still prevents reuse of its tag name.

## Release-QA checklist (§27)

⚙ = CI-enforced; listed so a human confirms the enforcement actually ran.

1. ⚙ Every applicable suite-`0x02` vector green on the release commit (`ci` + release gate).
2. ⚙ `spec-drift` green — vendored SPEC/vectors/wordlist match the pinned
   upstream (`.github/workflows/spec-drift.yml`).
3. ⚙ Pinned real-Lite interoperability green in all three official-client directions.
4. ⚙ **No-phone-home invariant**: zero egress to any host other than the selected API
   host across the matrix.
5. ⚙ Size gate ≤ 9 MiB × 6 targets; import allowlist green
   (`scripts/check-size.sh`, `scripts/check-deps.sh`).
6. ⚙ `govulncheck` clean; toolchain at the latest patch of its minor.
7. Cosign bundle verifies with the README one-liner *as a user would run it*,
   on a machine that is not the release runner.
8. `repro-verify` green (announcement waits for it).
9. CHANGELOG entry present; tag-derived `burnerpad version` output correct on a
   tap-installed binary, not a dev build.
10. NOTICE present in: archive, deb/rpm/apk, `burnerpad licenses` output.
