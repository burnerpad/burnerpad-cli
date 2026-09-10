# Burnerpad Envelope Spec — v1

> **Status:** normative, v1.
> **This document plus [`vectors/v1.json`](vectors/v1.json) are the source of truth.** The shipping
> implementation ([`burnerpad-crypto.js`](burnerpad-crypto.js)) and any other client (a CLI, a mobile app,
> an SDK in another language) **conform to this spec and these vectors** — not to any one implementation.
>
> **Scope.** How a secret is encrypted client-side, what bytes the server stores, and what travels in the
> URL fragment — the contract that guarantees a link created by one client decrypts in another. Validate
> with `npm test`.

---

## 1. Design invariants

1. **The server is crypto-agnostic.** It stores and relays one **opaque blob** and never parses it. All
   format knowledge lives in the client.
2. **The fragment is the key, and never reaches the server.** Browsers never transmit the URL fragment.
   Official clients must not log fragments, phrases, secret IDs/links, management tokens, ciphertext, or
   plaintext. (Suite `0x02` carries no fragment — the passphrase is shared out of band.)
3. **Fresh official-client credentials.** Every `0x01` secret gets a fresh random 256-bit key used to encrypt exactly one
   plaintext, which is what makes a random 96-bit GCM IV safe (the catastrophic (key, nonce)-reuse mode of
   GCM is structurally impossible). For `0x02`, a fresh random salt per secret gives a fresh derived key;
   official clients also generate a unique phrase per secret because the suite does not bind a server ID.
4. **Fail closed.** Any authentication failure, unknown suite, or malformed input is a hard reject — the
   client never shows partial or unauthenticated plaintext.
5. **Extensible by `suite`, not by guesswork.** A one-byte `suite` discriminator selects the entire
   format. A suite id is **never redefined**; a blob is valid forever under the suite it was created with.

---

## 2. Suite registry

The first envelope byte is `suite`. It uniquely identifies the format, globally and for all time.

| `suite` | Name | Construction | Key source | Status |
|---|---|---|---|---|
| `0x01` | `SYMMETRIC_AESGCM_V1` | AES-256-GCM | random 256-bit key in the URL fragment | **Defined (§3)** |
| `0x02` | `PASSPHRASE_PBKDF2_AESGCM_V1` | AES-256-GCM | PBKDF2-HMAC-SHA256(passphrase, salt, 600000) | **Defined (§4)** |
| `0x03`–`0xFF` | — | reserved (e.g. sealed-box X25519) | — | Reserved |

A client that reads an unknown `suite` **MUST** reject with `reject_unsupported_suite` (never guess).

Common parameters: IV/nonce = 96-bit (12 bytes) from a CSPRNG, fresh per secret; auth tag = 128-bit
(16 bytes), appended to the ciphertext per the WebCrypto convention; no compression (plaintext length minus
AEAD overhead is observable — accepted, to avoid compression-oracle bugs).

---

## 3. Suite `0x01` — `SYMMETRIC_AESGCM_V1`

```
blob = 0x01 ‖ iv(12) ‖ ciphertext‖tag
fragment = base64url_unpadded( key[32] )
aad = 0x01 0x01
```

- `key` is a single-use 256-bit CSPRNG value, carried **only** in the URL fragment.
- A decoder **MUST** validate the decoded key is exactly 32 bytes (`reject_bad_key` otherwise) and that the
  fragment is **canonical** base64url — no padding (`=`), no `+`/`/`, no whitespace, nothing outside
  `[A-Za-z0-9_-]`, and no non-canonical trailing bits (`reject_bad_encoding` otherwise). The simplest
  conformant test is to **re-encode the decoded bytes and require byte-equality with the input**, which
  admits exactly one encoding per key. Canonicality keeps independent clients in agreement (a lenient `atob`
  would accept a re-padded or trailing-bit-dirty link another client rejects, so the link would open in one
  and fail in the other). **Note:** Go's `base64.RawURLEncoding.DecodeString` is *not* sufficient on its own
  — it does not reject non-canonical trailing bits and it silently skips embedded `\r`/`\n`; a Go client
  must add the re-encode-and-compare check (or use `RawURLEncoding.Strict()` **and** reject newlines).

## 4. Suite `0x02` — `PASSPHRASE_PBKDF2_AESGCM_V1`

```
blob = 0x02 ‖ salt(16) ‖ iv(12) ‖ ciphertext‖tag
fragment = ""                            (none — the passphrase is shared out of band)
key = PBKDF2-HMAC-SHA256(passphrase_utf8, salt, iterations=600000, dkLen=32)
aad = 0x02 0x02 ‖ salt ‖ iv              (the entire header is authenticated)
```

- `salt` is a fresh 16-byte CSPRNG value per secret. The passphrase is UTF-8 encoded before derivation,
  with **no Unicode normalization** — implementations MUST use the raw UTF-8 bytes, so the derived key is
  identical across implementations regardless of the engine's Unicode version. (Consequence: a passphrase
  in a different normalization form will not decrypt; clients SHOULD advise ASCII passphrases. Passphrase
  *strength* is an application concern, out of scope for this format.)
- The shipping JavaScript API accepts only a primitive JavaScript string for `passphrase`; it MUST reject
  every other type with `reject_bad_passphrase` instead of relying on implicit `TextEncoder` coercion. An
  empty string remains format-valid; enforcing passphrase strength belongs to the calling application.
- Binding `salt` and `iv` into the AAD authenticates the whole header: tampering with either fails GCM
  authentication (`auth_fail`) rather than yielding wrong-but-accepted plaintext.

---

## 5. Additional Authenticated Data (AAD)

```
suite 0x01:  aad = suite ‖ spec_version              = 0x01 0x01
suite 0x02:  aad = suite ‖ spec_version ‖ salt ‖ iv   = 0x02 0x02 ‖ salt(16) ‖ iv(12)
```

`spec_version` is determined by the `suite` via the registry (§2), not by the implementation's own version,
so any conformant client constructs the identical AAD from the blob alone. Binding `suite`+`spec_version`
defeats **downgrade / suite-confusion**: a ciphertext cannot be reinterpreted under another suite without
GCM authentication failing. The `id` is deliberately **not** bound because the client does not know it at
encrypt time. With unique phrases, swapping records fails authentication. If a phrase is reused, however,
a server can substitute another valid suite-`0x02` blob created with that phrase and the client will
authenticate and return the other plaintext. Callers MUST NOT reuse suite-`0x02` phrases across secrets.

---

## 6. Procedures (normative)

```
ENCRYPT 0x01                                  ENCRYPT 0x02
1. key = CSPRNG(32); iv = CSPRNG(12)          1. salt = CSPRNG(16); iv = CSPRNG(12)
2. ct_tag = AESGCM(key, iv, pt, 0x01 0x01)    2. key = PBKDF2-SHA256(pass, salt, 600000, 32)
3. blob = 0x01 ‖ iv ‖ ct_tag                  3. aad = 0x02 0x02 ‖ salt ‖ iv
4. fragment = base64url(key)                  4. ct_tag = AESGCM(key, iv, pt, aad)
                                              5. blob = 0x02 ‖ salt ‖ iv ‖ ct_tag

DECRYPT
1. suite = blob[0]; if not in registry -> reject_unsupported_suite
2. if blob too short for the suite -> reject_truncated
3. (0x01) key = base64url_decode(fragment); if len != 32 -> reject_bad_key
   (0x02) key = PBKDF2-SHA256(passphrase, salt_from_blob, 600000, 32)
4. plaintext = AESGCM_Decrypt(key, iv, ct_tag, aad);  if auth fails -> auth_fail
```

### Canonical reject reasons

| Reason | When |
|---|---|
| `reject_unsupported_suite` | first byte is not a defined suite |
| `reject_truncated` | blob shorter than the suite's minimum header + tag |
| `reject_bad_key` | (0x01) decoded fragment key is not exactly 32 bytes |
| `reject_bad_encoding` | (0x01) fragment is not strict base64url |
| `reject_bad_passphrase` | (JavaScript API) passphrase is not a primitive string |
| `auth_fail` | GCM authentication failed — wrong key/passphrase, or tampered blob/header |

Reject **precedence** is not pinned in v1; conformance vectors are **single-fault** (each negative triggers
exactly one reason) so implementations may order their structural checks differently and still agree.

---

## 7. Test vectors & conformance

[`vectors/v1.json`](vectors/v1.json) is **language-neutral** (hex fields) and **committed static data**,
generated deterministically by [`tools/gen_vectors.mjs`](tools/gen_vectors.mjs) from fixed fixtures — never
at test time. **Conformance = green on the entire set.**

| Class | Proves |
|---|---|
| **decrypt KAT** | a fixed blob decrypts to the known plaintext (the cross-impl anchor) |
| **encrypt KAT** | encryption with a fixed key/iv (and passphrase/salt) reproduces the exact blob |
| **negative** | tampered tag/ciphertext/salt, flipped AAD, wrong key/passphrase, short key, bad encoding, truncation, unknown suite — all fail closed with the canonical reason |
| **encoding** | the fragment is base64url(key), URL-safe, no padding, 32 bytes |

- [`tools/selftest.mjs`](tools/selftest.mjs) checks the vectors against an independent reference on **two
  backends** (WebCrypto and node:crypto/OpenSSL), proving the format is implementation-independent.
- [`test/conformance.mjs`](test/conformance.mjs) checks the vectors against the **shipping bundle**. Because
  the bundle exposes no fixed-IV entrypoint (so it can never be coerced into nonce reuse), its *encrypt* path
  is verified by letting it choose its own randomness and re-encrypting with the reference at the same
  iv/salt, requiring byte-equality — encrypt conformance with **no nonce-reuse footgun shipped to browsers**.

A new implementation in any language is conformant iff it (a) decrypts every decrypt-KAT blob to the
expected plaintext, (b) reproduces every encrypt-KAT `expected_blob` given the fixed inputs, and (c) fails
every negative with the canonical reason.

---

## 8. Versioning policy

`suite` ids are globally unique and **never redefined**; a blob is valid forever under its suite. A new mode
is a new `suite` id (and may define a new `spec_version`); multiple suites coexist and a client dispatches on
the `suite` byte, not on a "spec era". Changes are recorded in [`CHANGELOG.md`](CHANGELOG.md).

## 9. Out of scope

`<id>` format, TTL/expiry, management-token/revoke, the HTTP/API surface, and burn-on-read semantics are
server concerns (see the server repo). Key verification / identity / sealed-box are future suites. Endpoint
compromise and the "link is the credential" property are mitigated at the app layer (burn-on-read, client
integrity), not in this format.

---

*Apache-2.0. Copyright (C) 2026 Impulsa SLU.*
