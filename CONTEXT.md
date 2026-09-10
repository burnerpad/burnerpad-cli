# Burnerpad CLI Context

The Burnerpad CLI is an independently distributed client for creating and revealing one-claim secrets
through the current burnerpad-lite service contract.

## Language

**Server contract**:
The current, unversioned HTTP and cryptographic contract maintained by burnerpad-lite. Retired pre-1.0
endpoints and behaviors are outside the CLI's compatibility boundary.
_Avoid_: Legacy API, old server, v1 API

**Server origin**:
The scheme, host, and optional port identifying the Burnerpad service used by a network operation.
_Avoid_: Backend, API URL, base URL

**Share link**:
A keyless URL containing the server origin and canonical secret identifier. Possession permits an attempt to
claim and destroy the ciphertext, but the separate phrase is required to decrypt it.
_Avoid_: Secret URL, decryption link

**Official client**:
The burnerpad-lite browser or an independently distributed Burnerpad CLI that follows the same current
service and secret-creation contract.
_Avoid_: Reference implementation

**Cross-client compatible secret**:
A passphrase secret that can be created by either official client and revealed by either official client.
Official clients create these with suite `0x02` and a keyless share link.
_Avoid_: CLI secret, browser secret

**Secret identifier**:
The canonical 26-character uppercase Crockford-base32 identifier assigned by the server. User input may use
the server's lowercase, separator, and `I`/`L`/`O` aliases before normalization.
_Avoid_: Short ID, token

**Generated phrase**:
A fresh phrase of exactly seven distinct words selected uniformly by an official client from the shared EFF
wordlist. Its strength claim does not apply to words selected by a person.
_Avoid_: Password, recovery phrase

**Supplied phrase**:
A user-selected phrase containing 7–64 distinct words from the shared EFF wordlist. Input formatting is
canonicalized before encryption, but the selected words and their order are unchanged.
_Avoid_: Custom password, generated phrase

**Canonical phrase**:
The cryptographic representation of a generated or supplied phrase: lowercase shared-list words joined by
single ASCII spaces, with no leading or trailing whitespace.
_Avoid_: Raw phrase, normalized password

**Create receipt**:
The structured machine result containing the share link, phrase, management token, effective lifetime, and
server origin for one successfully created secret.
_Avoid_: Secret, response dump

**Text secret**:
Well-formed UTF-8 plaintext accepted and produced by official clients without Unicode normalization.
_Avoid_: Binary secret, normalized text

**Management token**:
A destroy-only capability returned once when a secret is created. It can authorize revocation but cannot
claim or decrypt the secret.
_Avoid_: API token, decryption key

**Claim**:
The atomic server operation that removes a live ciphertext row and makes its blob available to at most one
request handler. A claim does not guarantee delivery or decryption.
_Avoid_: Burning GET, exactly-once read, take

**Reveal**:
The client flow that claims a blob and attempts to decrypt it locally for the recipient.
_Avoid_: Fetch, read

**Revoke**:
The authenticated removal of a live ciphertext row without claiming or decrypting it.
The sole user-facing command for this operation is `burnerpad burn`; `revoke` is not a command or alias.
_Avoid_: Delete, using “burn” for a claim/reveal

**Unavailable secret**:
A secret for which the server returns the common unavailable response, without distinguishing whether it was
claimed, revoked, expired, purged, lost on restart, or never existed.
_Avoid_: Gone secret, missing secret

**Outcome unknown**:
The result of a network operation that may have reached the server but did not produce a complete, valid
response. The CLI does not turn this state into a second network attempt automatically.
_Avoid_: Network error, retryable failure

**Local retry**:
Another decryption attempt against a blob already held by the client. It performs no network operation and
cannot claim a secret again.
_Avoid_: Reveal retry, claim retry

**Recovery blob**:
Claimed ciphertext explicitly saved by the recipient before leaving the reveal flow so it can be decrypted
locally later. It contains no phrase or plaintext.
_Avoid_: Backup secret, error blob

**Effective lifetime**:
The whole number of seconds returned by the server for a created secret, after any server-side defaulting or
clamping. It is the authoritative lifetime shown to the user and returned in a create receipt.
_Avoid_: Sent TTL, requested TTL
