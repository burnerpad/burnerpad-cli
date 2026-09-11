// Adversarial hardening of the crypto core against the Wycheproof project's
// vector sets (vendored under testdata/wycheproof/, provenance in its
// README.md). White-box, same-package on purpose: the AES-GCM cases carry
// arbitrary per-case AAD, so they are driven through the unexported
// open(key, iv, ct‖tag, aad) — the exact call shape both suites use — and
// the PBKDF2 cases are driven through the production deriveKey via the
// kdfIter test seam, so every assertion exercises the shipped code path,
// not a test-local reimplementation.
//
// Vendored-file discipline mirrors testdata/v1.json: SHA-256-pinned,
// strictly decoded (DisallowUnknownFields), and every selection/result count
// is pinned so an upstream re-vendor can never silently shrink coverage or
// smuggle in a case class this file has no policy for.
package envelope

import (
	"bytes"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Provenance: github.com/C2SP/wycheproof @ commit
// 4bb5ed764261bb3699f93567998a3467d3cc9785 (main, 2026-08-17; the repo
// publishes no release tags), files testvectors_v1/{aes_gcm_test.json,
// pbkdf2_hmacsha256_test.json}, byte-identical to upstream. License:
// Apache-2.0. See testdata/wycheproof/README.md. Re-pin only via review.
const (
	wycheproofCommit       = "4bb5ed764261bb3699f93567998a3467d3cc9785"
	wycheproofGCMSHA256    = "985e5ecc172e181eaf49e89508b9470dcf478002eb7e8559c707eb42dc97dfe7"
	wycheproofPBKDF2SHA256 = "1bf37af2cefe40c829ee9ecebb3505bb6424be8824bc97aa3e1c2076e860d192"
)

// ---- strict Wycheproof schema (fields cover the two vendored files
// exactly; anything unknown fails the decode, and the sha256 pin means the
// bytes cannot drift without a reviewed re-pin anyway).

type wpNote struct {
	BugType     string   `json:"bugType"`
	Description string   `json:"description"`
	Effect      string   `json:"effect"`
	CVEs        []string `json:"cves"`
}

type wpSource struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type wpGCMFile struct {
	Algorithm     string            `json:"algorithm"`
	Schema        string            `json:"schema"`
	NumberOfTests int               `json:"numberOfTests"`
	Header        []string          `json:"header"`
	Notes         map[string]wpNote `json:"notes"`
	TestGroups    []wpGCMGroup      `json:"testGroups"`
}

type wpGCMGroup struct {
	Type    string      `json:"type"`
	Source  wpSource    `json:"source"`
	KeySize int         `json:"keySize"` // bits
	IVSize  int         `json:"ivSize"`  // bits
	TagSize int         `json:"tagSize"` // bits
	Tests   []wpGCMTest `json:"tests"`
}

type wpGCMTest struct {
	TcID    int      `json:"tcId"`
	Comment string   `json:"comment"`
	Flags   []string `json:"flags"`
	Key     string   `json:"key"` // hex
	IV      string   `json:"iv"`  // hex
	AAD     string   `json:"aad"` // hex
	Msg     string   `json:"msg"` // hex
	CT      string   `json:"ct"`  // hex
	Tag     string   `json:"tag"` // hex
	Result  string   `json:"result"`
}

type wpPBKDF2File struct {
	Algorithm     string            `json:"algorithm"`
	Schema        string            `json:"schema"`
	NumberOfTests int               `json:"numberOfTests"`
	Header        []string          `json:"header"`
	Notes         map[string]wpNote `json:"notes"`
	TestGroups    []wpPBKDF2Group   `json:"testGroups"`
}

type wpPBKDF2Group struct {
	Type   string         `json:"type"`
	Source wpSource       `json:"source"`
	Tests  []wpPBKDF2Test `json:"tests"`
}

type wpPBKDF2Test struct {
	TcID           int      `json:"tcId"`
	Comment        string   `json:"comment"`
	Flags          []string `json:"flags"`
	Password       string   `json:"password"` // hex (raw bytes; NonUtf8 cases exist)
	Salt           string   `json:"salt"`     // hex
	IterationCount int      `json:"iterationCount"`
	DKLen          int      `json:"dkLen"` // bytes
	DK             string   `json:"dk"`    // hex
	Result         string   `json:"result"`
}

// wycheproofLoad reads a vendored file, verifies its pinned sha256, and
// strictly decodes it into out.
func wycheproofLoad(t *testing.T, name, pin string, out any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "wycheproof", name))
	if err != nil {
		t.Fatalf("read vendored wycheproof file: %v", err)
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != pin {
		t.Fatalf("vendored %s drifted: sha256 = %s, pinned %s — re-vendor + re-pin only via review (commit %s)",
			name, got, pin, wycheproofCommit)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		t.Fatalf("strict decode of %s: %v", name, err)
	}
	if dec.More() {
		t.Fatalf("trailing data after %s", name)
	}
}

// TestWycheproofAESGCM drives every Wycheproof AES-GCM case matching the
// envelope-v1 profile — keySize 256 ∧ ivSize 96 ∧ tagSize 128, the only GCM
// shape either suite can produce or accept — through the unexported open()
// with the case's own AAD (the suite-0x01-shaped call: open(key, iv,
// ct‖tag, aad)).
//
// Expected outcomes:
//   - "valid"      → plaintext returned and equal to msg;
//   - "invalid"    → exactly ErrAuthFail, no plaintext (fail closed);
//   - "acceptable" → MUST-REJECT (ErrAuthFail). Rationale: our profile is
//     exact-parameter GCM — Wycheproof marks a case "acceptable" only when
//     it exercises edge behaviors (legacy or borderline parameters, lenient
//     decodings) that a strict implementation is entitled to refuse, and
//     this package refuses all of them. At the pinned commit the selection
//     contains zero such cases; the pinned count below turns any future
//     appearance into a loud, reviewed decision instead of a silent accept.
//
// Out-of-profile groups (other key/IV/tag sizes) are counted but not run:
// they exercise GCM shapes mustGCM can never construct — feeding them
// through open() would test a different cipher configuration than the one
// that ships.
func TestWycheproofAESGCM(t *testing.T) {
	var f wpGCMFile
	wycheproofLoad(t, "aes_gcm_test.json", wycheproofGCMSHA256, &f)

	if f.Algorithm != "AES-GCM" {
		t.Fatalf("algorithm = %q, want AES-GCM", f.Algorithm)
	}
	if f.Schema != "aead_test_schema_v1.json" {
		t.Fatalf("schema = %q, want aead_test_schema_v1.json", f.Schema)
	}

	total := 0
	run, valid, invalid, acceptable := 0, 0, 0, 0
	for _, g := range f.TestGroups {
		if g.Type != "AeadTest" {
			t.Fatalf("unexpected group type %q", g.Type)
		}
		total += len(g.Tests)
		if g.KeySize != 256 || g.IVSize != 96 || g.TagSize != 128 {
			continue // out of profile, deliberately not run (see doc comment)
		}
		for _, tc := range g.Tests {
			tc := tc
			t.Run(fmt.Sprintf("tc%03d", tc.TcID), func(t *testing.T) {
				key := mustHex(t, tc.Key)
				iv := mustHex(t, tc.IV)
				aad := mustHex(t, tc.AAD)
				msg := mustHex(t, tc.Msg)
				ct := mustHex(t, tc.CT)
				tag := mustHex(t, tc.Tag)

				// The group declares the sizes; hold every case to them so a
				// malformed vector can never route a wrong-shape call (which
				// would panic inside GCM rather than test anything).
				if len(key) != KeyLen || len(iv) != IVLen || len(tag) != TagLen {
					t.Fatalf("tc%d: case sizes key=%d iv=%d tag=%d disagree with group 256/96/128",
						tc.TcID, len(key)*8, len(iv)*8, len(tag)*8)
				}

				ctTag := make([]byte, 0, len(ct)+len(tag))
				ctTag = append(ctTag, ct...)
				ctTag = append(ctTag, tag...)

				pt, err := open(key, iv, ctTag, aad)
				switch tc.Result {
				case "valid":
					if err != nil {
						t.Fatalf("tc%d (%s): open() = %v, want plaintext", tc.TcID, tc.Comment, err)
					}
					if !bytes.Equal(pt, msg) {
						t.Fatalf("tc%d (%s): plaintext mismatch\n got %x\nwant %x", tc.TcID, tc.Comment, pt, msg)
					}
				case "invalid", "acceptable": // acceptable = MUST-REJECT here (see doc comment)
					if !errors.Is(err, ErrAuthFail) {
						t.Fatalf("tc%d (%s, %s, flags %v): open() err = %v, want ErrAuthFail",
							tc.TcID, tc.Result, tc.Comment, tc.Flags, err)
					}
					if pt != nil {
						t.Fatalf("tc%d: plaintext returned alongside rejection", tc.TcID)
					}
				default:
					t.Fatalf("tc%d: unknown result %q", tc.TcID, tc.Result)
				}
			})
			run++
			switch tc.Result {
			case "valid":
				valid++
			case "invalid":
				invalid++
			case "acceptable":
				acceptable++
			}
		}
	}

	if total != f.NumberOfTests {
		t.Fatalf("parsed %d cases, file declares numberOfTests=%d", total, f.NumberOfTests)
	}
	// Selection counts pinned at commit 4bb5ed76: the whole 256/96/128
	// profile of the file, nothing skipped. Any deviation means the vendored
	// bytes changed (impossible without a re-pin) or the filter regressed.
	if run != 66 || valid != 39 || invalid != 27 || acceptable != 0 {
		t.Fatalf("selection drifted: run=%d valid=%d invalid=%d acceptable=%d, want 66/39/27/0",
			run, valid, invalid, acceptable)
	}
	t.Logf("wycheproof AES-GCM: %d cases in file; ran all %d in the 256/96/128 profile through open() (%d valid, %d invalid, %d acceptable-as-reject)",
		total, run, valid, invalid, acceptable)
}

// TestWycheproofPBKDF2HMACSHA256 drives every Wycheproof PBKDF2-HMAC-SHA256
// case through both layers of the derivation stack:
//
//  1. stdlib conformance — crypto/pbkdf2.Key with sha256 at the case's exact
//     dkLen must reproduce dk bit-for-bit (this is the library deriveKey
//     stands on, so its conformance is worth pinning at every dkLen the set
//     covers, 65-byte cases included);
//  2. production-path conformance — deriveKey itself, with the kdfIter seam
//     set to the case's iteration count (saved/restored), fed the case's raw
//     password and salt bytes. deriveKey always emits KeyLen=32 bytes, and
//     RFC 8018 §5.2 makes PBKDF2 output a prefix of the T_1‖T_2‖… block
//     stream, so a 32-byte derivation must agree with the case's dk on their
//     overlap regardless of dkLen — dkLen==32 cases compare in full, the
//     task's dkLen≤64 applicability set is covered as a superset.
//
// Result policy: at the pinned commit all 60 cases are "valid" (counts
// pinned below). A hypothetical "invalid" case (per the pbkdf schema these
// would carry a wrong dk) must NOT be reproduced by the stack; an
// "acceptable" case appearing on a re-vendor fails the run outright — its
// flags would have to be reviewed and given an explicit policy here before
// the pin is updated. Iteration counts are honored at full fidelity (max in
// the set: 80000), never truncated.
func TestWycheproofPBKDF2HMACSHA256(t *testing.T) {
	var f wpPBKDF2File
	wycheproofLoad(t, "pbkdf2_hmacsha256_test.json", wycheproofPBKDF2SHA256, &f)

	if f.Algorithm != "PBKDF2-HMACSHA256" {
		t.Fatalf("algorithm = %q, want PBKDF2-HMACSHA256", f.Algorithm)
	}
	if f.Schema != "pbkdf_test_schema.json" {
		t.Fatalf("schema = %q, want pbkdf_test_schema.json", f.Schema)
	}

	prev := kdfIter
	defer func() { kdfIter = prev }()

	total, valid, invalid, acceptable := 0, 0, 0, 0
	dk32Direct, dkLE64 := 0, 0
	for _, g := range f.TestGroups {
		if g.Type != "PbkdfTest" {
			t.Fatalf("unexpected group type %q", g.Type)
		}
		for _, tc := range g.Tests {
			tc := tc
			t.Run(fmt.Sprintf("tc%03d", tc.TcID), func(t *testing.T) {
				password := mustHex(t, tc.Password)
				salt := mustHex(t, tc.Salt)
				dk := mustHex(t, tc.DK)
				if len(dk) != tc.DKLen {
					t.Fatalf("tc%d: len(dk)=%d disagrees with dkLen=%d", tc.TcID, len(dk), tc.DKLen)
				}
				if tc.IterationCount < 1 {
					t.Fatalf("tc%d: iterationCount=%d", tc.TcID, tc.IterationCount)
				}

				// Layer 1: stdlib at the exact dkLen.
				std, err := pbkdf2.Key(sha256.New, string(password), salt, tc.IterationCount, tc.DKLen)
				if err != nil {
					t.Fatalf("tc%d: stdlib pbkdf2.Key: %v", tc.TcID, err)
				}

				// Layer 2: the production deriveKey through the seam.
				kdfIter = tc.IterationCount
				derived := deriveKey(password, salt)
				kdfIter = prev

				switch tc.Result {
				case "valid":
					if !bytes.Equal(std, dk) {
						t.Fatalf("tc%d (%s, flags %v): stdlib mismatch\n got %x\nwant %x",
							tc.TcID, tc.Comment, tc.Flags, std, dk)
					}
					overlap := min(len(dk), KeyLen)
					if !bytes.Equal(derived[:overlap], dk[:overlap]) {
						t.Fatalf("tc%d (%s): deriveKey disagrees with dk on the first %d bytes\n got %x\nwant %x",
							tc.TcID, tc.Comment, overlap, derived[:overlap], dk[:overlap])
					}
				case "invalid":
					// None at the pinned commit. Policy: the dk of an invalid
					// case is by definition wrong, so the stack must NOT
					// reproduce it.
					if bytes.Equal(std, dk) {
						t.Fatalf("tc%d: stack reproduced the dk of an invalid case", tc.TcID)
					}
				case "acceptable":
					t.Fatalf("tc%d: 'acceptable' PBKDF2 case appeared (flags %v) — review its flags and give it an explicit policy before re-pinning",
						tc.TcID, tc.Flags)
				default:
					t.Fatalf("tc%d: unknown result %q", tc.TcID, tc.Result)
				}
			})
			total++
			switch tc.Result {
			case "valid":
				valid++
			case "invalid":
				invalid++
			case "acceptable":
				acceptable++
			}
			if tc.DKLen == KeyLen {
				dk32Direct++
			}
			if tc.DKLen <= 64 {
				dkLE64++
			}
		}
	}

	if total != f.NumberOfTests {
		t.Fatalf("parsed %d cases, file declares numberOfTests=%d", total, f.NumberOfTests)
	}
	// Counts pinned at commit 4bb5ed76.
	if total != 60 || valid != 60 || invalid != 0 || acceptable != 0 {
		t.Fatalf("case counts drifted: total=%d valid=%d invalid=%d acceptable=%d, want 60/60/0/0", total, valid, invalid, acceptable)
	}
	if dk32Direct != 4 || dkLE64 != 44 {
		t.Fatalf("dkLen distribution drifted: dkLen==32: %d (want 4), dkLen<=64: %d (want 44)", dk32Direct, dkLE64)
	}
	t.Logf("wycheproof PBKDF2-HMAC-SHA256: %d cases — all driven through stdlib pbkdf2.Key at exact dkLen AND through deriveKey via the kdfIter seam (%d full 32-byte compares, %d within the dkLen<=64 applicability set, max iteration count honored in full)",
		total, dk32Direct, dkLE64)
}
