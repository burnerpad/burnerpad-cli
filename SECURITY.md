# Security

## Reporting

Email **security@burnerpad.io**. Do not open a public issue for a vulnerability. We aim to acknowledge a
report within 72 hours and coordinate disclosure through GitHub Security Advisories.

## Guarantees

- Plaintext is encrypted locally with suite `0x02`; the server receives only opaque ciphertext.
- Share URLs contain no key. The passphrase travels out of band and never enters a request.
- Create, claim, and revoke each send at most one operation request per invocation.
- A claim validates its URL, phrase, output file, recovery file, and clipboard destination first.
- A wrong phrase is retried locally against held ciphertext and cannot issue another claim.
- Remote servers require verified HTTPS, loopback alone may use HTTP, and redirects are refused.
- Passphrases, tokens, and plaintext have no argv/environment interface.
- There is no config file, persistent state, telemetry, crash reporter, update check, or compatibility probe.
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
share URL supplied in argv, so the CLI warns and supports prompt/pipe URL input. Clipboard managers may keep
history after OSC 52 clearing. Plain terminal mode can place plaintext in scrollback.

### Go memory hygiene

Secrets use byte slices and are wiped after their last controlled use; core-dump/debugger hardening and page
locking are best effort. Go's collector, stack movement, cryptographic key schedules, terminal drivers,
kernel pipes, swap, and third-party clipboard storage can retain copies the process cannot erase. These
measures narrow exposure; they do not make a garbage-collected process leak-proof.

## Release verification

Releases attach checksums, SBOMs, and a keyless Sigstore bundle. Publishing
also creates an immutable-release attestation binding the tag, commit, and
complete asset set. The signing workflow is loaded from the default branch and
pins the release to the dispatched commit:

```sh
cosign verify-blob --bundle SHA256SUMS.bundle \
  --certificate-identity 'https://github.com/Cinderella-Man/burnerpad-cli/.github/workflows/publish-release.yml@refs/heads/main' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
```

That exact repository/workflow identity is a release trust root. Change it in
the repository-transfer commit before publishing from a different owner.

CI gates the current suite-`0x02` vectors and real browser/CLI interoperability against a reviewed
burnerpad-lite revision. A scheduled run tests the same matrix against Lite `main` to expose drift early.
