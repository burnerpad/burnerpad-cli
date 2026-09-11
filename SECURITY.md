# Security

## Reporting

Email **security@burnerpad.io**. Do not open a public issue for a vulnerability. We aim to acknowledge a
report within 72 hours and coordinate disclosure through GitHub Security Advisories.

## Guarantees

- Plaintext is encrypted locally with suite `0x02`; the server receives only opaque ciphertext.
- Share URLs contain no key. The passphrase travels out of band and never enters a request.
- Create, claim, and revoke each send at most one operation request per invocation.
- The first termination signal cancels and joins an in-flight command. A transmitted mutation left
  unconfirmed is reported as outcome unknown, and required terminal cleanup is attempted before the process
  returns.
- Confirmed one-time plaintext that becomes ready after cancellation is not flashed and erased: terminal
  delivery falls back to persistent plain rendering.
- A claim validates its URL, phrase, output file, recovery file, and terminal destination first.
- A wrong phrase is retried locally against held ciphertext and cannot issue another claim.
- Remote servers require verified HTTPS, loopback alone may use HTTP, and redirects are refused.
- Passphrases, tokens, and plaintext have no argv/environment interface.
- Named passphrase and management-token files are opened without following a final symlink/reparse point and
  validated on the opened handle as current-user-owned, owner-only regular files before any network request;
  macOS extended ACLs are refused because they can grant access outside the BSD mode bits.
- Plaintext and recovery outputs are reserved with exclusive owner-only creation and never overwrite an
  existing path; macOS atomically suppresses inherited ACLs, and Windows installs a protected owner-only DACL.
- Both interactive viewer modes treat plaintext as untrusted display data and visibly escape terminal
  controls and Unicode format characters. Pipe and file destinations retain the authenticated bytes, and
  decoded JSON round-trips them.
- There is no built-in clipboard integration: unverifiable terminal clipboard protocols and PATH-resolved
  helper programs are outside the trusted destination boundary.
- There is no config file, persistent state, telemetry, crash reporter, update check, or compatibility probe.
- Pinned, scheduled-main, and release interoperability run the Linux real-client process tree behind
  self-tested per-origin IPv4/IPv6 egress rules; any unexpected observed IP packet fails the gate.
- Machine and human errors exclude URLs, IDs, phrases, tokens, blobs, plaintext, response bodies, and paths.

Use a fresh phrase for every secret. Suite `0x02` does not bind the server-assigned ID into authenticated
data, so reusing a phrase would let a malicious server substitute a different valid blob created with that
same phrase. The default generator makes a fresh random selection; callers supplying one are responsible for its
uniqueness.

An incomplete or invalid response after a mutation may have reached the server is reported as outcome
unknown. That is deliberately not converted into a retry or a comforting guess.

## Honest limits

Endpoint malware, keyloggers, compromised terminals, and a compromised live web client defeat end-to-end
protection. A caller-supplied phrase is only as strong as the caller's selection. Shell history records a
share URL supplied in argv, so the CLI warns and supports prompt/pipe URL input. Plain terminal mode can place
the safe, escaped rendition in scrollback; the alternate screen only reduces primary-scrollback exposure. A
compromised terminal, or a caller that replays byte-exact pipe, file, or decoded-JSON output to a terminal,
remains out of scope. Callers that pipe plaintext into an external clipboard tool own its integrity and
retention behavior.
Credential-file metadata cannot protect against a compromised current account, root/administrator access,
or a filesystem/kernel that reports false ownership or access-control data. Ancestor directory symlinks are
permitted; the opened object itself is still validated without a path/stat race.
Intrinsically blocked operating-system I/O may delay graceful termination; a second signal restores immediate
OS termination and can bypass best-effort terminal or file cleanup. SIGKILL cannot run cleanup.

### Go memory hygiene

Secrets use byte slices and are wiped after their last controlled use; core-dump/debugger hardening and page
locking are best effort. Go's collector, stack movement, cryptographic key schedules, terminal drivers,
kernel pipes, swap, and third-party destinations can retain copies the process cannot erase. These measures
narrow exposure; they do not make a garbage-collected process leak-proof.

## Release verification

Releases attach checksums, SBOMs, and a keyless Sigstore bundle. Publishing
also creates an immutable-release attestation binding the tag, commit, and
complete asset set. The signing workflow is loaded from the default branch and
pins the release to the dispatched commit:

```sh
cosign verify-blob --bundle SHA256SUMS.bundle \
  --certificate-identity 'https://github.com/burnerpad/burnerpad-cli/.github/workflows/publish-release.yml@refs/heads/main' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
```

That exact repository/workflow identity is a release trust root. It must be
changed in the repository-transfer commit before publishing from another
owner.

CI pins and executes the current suite-`0x02` and generic encoding vectors, byte-checks the vendored crypto
material, and gates real browser/CLI interoperability against a reviewed burnerpad-lite revision. The
`lite-main-interop` workflow is configured to test the same matrix against Lite `main` on a schedule to
expose drift early.
