// Published PBKDF2-HMAC-SHA256 known-answer tests, asserted through the
// production deriveKey with the kdfIter seam. Belt and braces on top of the
// Wycheproof set: deriveKey is the ONE derivation code path in the package —
// EncryptPassphrase and DecryptPassphrase both call it with kdfIter at its
// shipped default of 600k — so KATs that pass through deriveKey (seam
// lowered to the vector's count) verify exactly the code the 600k
// production path runs, and TestRFCKATProductionIterationPath closes the
// loop by pinning the seam's default AND a full-fidelity 600k derivation.
package envelope

import (
	"bytes"
	"crypto/pbkdf2"
	"crypto/sha256"
	"testing"
)

// rfcKATs are the published PBKDF2-HMAC-SHA256 vectors: the SHA-256
// analogue of RFC 6070's password/salt/iteration set (circulated in IETF
// draft test-vector collections and pinned by countless implementations'
// suites), the long-input case, and the embedded-NUL case. RFC 7914 §11's
// two PBKDF2-HMAC-SHA256 vectors are covered by the Wycheproof set (tc 1–2).
// Every dk below was independently re-derived with Python 3
// hashlib.pbkdf2_hmac at vendoring time — no Go code involved — so this
// table cannot inherit a stdlib bug.
var rfcKATs = []struct {
	name     string
	password string
	salt     string
	iter     int
	dkLen    int
	dkHex    string
}{
	{"iter-1", "password", "salt", 1, 32,
		"120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"},
	{"iter-2", "password", "salt", 2, 32,
		"ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"},
	{"iter-4096", "password", "salt", 4096, 32,
		"c5e478d59288c841aa530db6845c4c8d962893a001ce4e11a4963873aa98134a"},
	{"long-input", "passwordPASSWORDpassword", "saltSALTsaltSALTsaltSALTsaltSALTsalt", 4096, 40,
		"348c89dbcbd32b2f32d814b8116e84cf2b17347ebc1800181c4e2a1fb8dd53e1c635518c7dac47e9"},
	{"embedded-nul", "pass\x00word", "sa\x00lt", 4096, 16,
		"89b69d0516f829893c696226650a8687"},
}

// TestRFCKATPBKDF2HMACSHA256 asserts every published KAT through deriveKey
// (kdfIter seam set to the vector's count, saved/restored). dkLen 32 cases
// compare the full output; the 40- and 16-byte cases use the RFC 8018 §5.2
// prefix property on the overlap AND pin the full-length dk via stdlib
// pbkdf2.Key, so both the production path and the library it stands on are
// held to the published bytes.
func TestRFCKATPBKDF2HMACSHA256(t *testing.T) {
	prev := kdfIter
	defer func() { kdfIter = prev }()

	for _, tc := range rfcKATs {
		t.Run(tc.name, func(t *testing.T) {
			dk := mustHex(t, tc.dkHex)
			if len(dk) != tc.dkLen {
				t.Fatalf("table bug: len(dk)=%d, dkLen=%d", len(dk), tc.dkLen)
			}

			kdfIter = tc.iter
			derived := deriveKey([]byte(tc.password), []byte(tc.salt))
			kdfIter = prev

			overlap := min(tc.dkLen, KeyLen)
			if !bytes.Equal(derived[:overlap], dk[:overlap]) {
				t.Fatalf("deriveKey disagrees with the published KAT on the first %d bytes\n got %x\nwant %x",
					overlap, derived[:overlap], dk[:overlap])
			}
			if tc.dkLen != KeyLen {
				std, err := pbkdf2.Key(sha256.New, tc.password, []byte(tc.salt), tc.iter, tc.dkLen)
				if err != nil {
					t.Fatalf("stdlib pbkdf2.Key: %v", err)
				}
				if !bytes.Equal(std, dk) {
					t.Fatalf("stdlib disagrees with the published %d-byte KAT\n got %x\nwant %x", tc.dkLen, std, dk)
				}
			}
		})
	}
}

// TestRFCKATProductionIterationPath closes the seam loop:
//
//  1. the SPEC constant is 600k and the seam's shipped default IS that
//     constant — so the code path every KAT above exercised is the
//     production path with nothing but the count swapped;
//  2. one full-fidelity KAT at the real 600k, seam untouched: the expected
//     bytes were independently derived with Python 3 hashlib.pbkdf2_hmac
//     ('sha256', b'password', b'salt', 600000, 32), so the exact derivation
//     a shipped binary performs is pinned against a non-Go implementation.
//     (~0.5 s once per run — the price of pinning the real parameters.)
func TestRFCKATProductionIterationPath(t *testing.T) {
	if Iterations != 600_000 {
		t.Fatalf("Iterations = %d, want 600000 (SPEC §4)", Iterations)
	}
	if kdfIter != Iterations {
		t.Fatalf("kdfIter seam = %d at rest, want Iterations (%d) — a test failed to restore it, or the shipped default drifted",
			kdfIter, Iterations)
	}

	derived := deriveKey([]byte("password"), []byte("salt"))
	want := mustHex(t, "669cfe52482116fda1aa2cbe409b2f56c8e4563752b7a28f6eaab614ee005178")
	if !bytes.Equal(derived, want) {
		t.Fatalf("600k production-path derivation mismatch\n got %x\nwant %x", derived, want)
	}
}
