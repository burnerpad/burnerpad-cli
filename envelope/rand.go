package envelope

import "crypto/rand"

// randRead is the ONLY randomness seam in the package. It is unexported and
// swapped exclusively by _test.go files in this same package (encrypt-KAT
// replay at fixed key/iv/salt, §14.1). No build of the shipped binary and no
// importer can ever reach it: the public API accepts only plaintext/passphrase
// and always draws key, iv, and salt itself. This mirrors the reference
// bundle's stance — burnerpad-crypto.js exposes no fixed-IV entrypoint either,
// so neither implementation can be coerced into GCM (key, nonce) reuse.
var randRead = rand.Read

// csprng returns n fresh bytes or panics. Go ≥ 1.24 guarantees
// crypto/rand.Read never returns an error (the runtime aborts if the kernel
// CSPRNG is unusable) — the check is kept so the invariant is enforced,
// not assumed.
func csprng(n int) []byte {
	b := make([]byte, n)
	if _, err := randRead(b); err != nil {
		panic("csprng unavailable: " + err.Error())
	}
	return b
}
