package envelope

import (
	"crypto/pbkdf2" // stdlib since Go 1.24 — why x/crypto is NOT in the graph
	"crypto/sha256"
	"unsafe"
)

// Iterations is fixed by SPEC §4 for suite 0x02. Not configurable, ever: a
// different count derives a different key and the blob simply fails auth.
const Iterations = 600_000

var kdfIter = Iterations // test seam for published PBKDF2/Wycheproof vectors; unexported and unreachable in a shipped binary

// deriveKey feeds PBKDF2 the passphrase bytes EXACTLY as given — raw UTF-8,
// no Unicode normalization, no trimming (SPEC §4; vector 02-nfd-passphrase
// pins the no-NFC behavior). crypto/pbkdf2 takes the password as a string; a
// plain string(pp) conversion would mint an immutable, unwipeable heap copy
// of the passphrase, so we alias the bytes zero-copy instead. Sound because
// pbkdf2.Key does not retain the string past the call and pp is not mutated
// during it; pp is wiped by its owner afterwards. This is the ONLY use of
// unsafe in the codebase — keep it that way.
func deriveKey(pp, salt []byte) []byte {
	alias := unsafe.String(unsafe.SliceData(pp), len(pp))
	key, err := pbkdf2.Key(sha256.New, alias, salt, kdfIter, KeyLen)
	if err != nil {
		// Reachable only under FIPS-mode parameter policing, which our fixed
		// parameters (16-byte salt, 32-byte dkLen) never trigger. Fail closed.
		panic("pbkdf2: " + err.Error())
	}
	return key
}
