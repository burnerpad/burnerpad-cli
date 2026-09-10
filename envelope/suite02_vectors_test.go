package envelope

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

type suite02VectorFile struct {
	Suites           []string                  `json:"suites"`
	Decrypt          []suite02DecryptVector    `json:"decrypt_kat"`
	Encrypt          []suite02EncryptVector    `json:"encrypt_kat"`
	Negative         []suite02NegativeVector   `json:"negative"`
	Encoding         []suite02EncodingVector   `json:"encoding"`
	EncodingNegative []suite02EncodingNegative `json:"encoding_negative"`
}

type suite02DecryptVector struct {
	Name          string  `json:"name"`
	Suite         string  `json:"suite"`
	Passphrase    *string `json:"passphrase"`
	PassphraseHex *string `json:"passphrase_hex"`
	Blob          string  `json:"blob"`
	Plaintext     *string `json:"plaintext"`
	Iter          int     `json:"iter"`
}

type suite02EncryptVector struct {
	Name          string  `json:"name"`
	Suite         string  `json:"suite"`
	Passphrase    *string `json:"passphrase"`
	PassphraseHex *string `json:"passphrase_hex"`
	Salt          string  `json:"salt"`
	IV            string  `json:"iv"`
	Plaintext     *string `json:"plaintext"`
	Expected      string  `json:"expected_blob"`
	Iter          int     `json:"iter"`
}

type suite02NegativeVector struct {
	Name          string  `json:"name"`
	Suite         string  `json:"suite"`
	Passphrase    *string `json:"passphrase"`
	PassphraseHex *string `json:"passphrase_hex"`
	Blob          *string `json:"blob"`
	Expect        string  `json:"expect"`
}

type suite02EncodingVector struct {
	Name     string  `json:"name"`
	Key      *string `json:"key"`
	Fragment *string `json:"fragment"`
}

type suite02EncodingNegative struct {
	Name     string  `json:"name"`
	Fragment *string `json:"fragment"`
	Expect   string  `json:"expect"`
}

func loadSuite02Vectors(t *testing.T) suite02VectorFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/v1.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if got := hex.EncodeToString(digest[:]); got != VectorsSHA256 {
		t.Fatalf("testdata/v1.json SHA-256 = %s, want %s", got, VectorsSHA256)
	}
	var vectors suite02VectorFile
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, suite := range vectors.Suites {
		if suite == "02" {
			found = true
		}
	}
	if !found {
		t.Fatal("vector registry no longer contains suite 02")
	}
	return vectors
}

func requiredVectorString(t *testing.T, name, field string, value *string) string {
	t.Helper()
	if value == nil {
		t.Fatalf("%s: missing %s", name, field)
	}
	return *value
}

func vectorPhrase(t *testing.T, name string, phrase, encoded *string) []byte {
	t.Helper()
	if (phrase == nil) == (encoded == nil) {
		t.Fatalf("%s: vector must have exactly one passphrase representation", name)
	}
	if encoded != nil {
		return mustHex(t, *encoded)
	}
	return []byte(*phrase)
}

func TestEverySuite02Vector(t *testing.T) {
	vectors := loadSuite02Vectors(t)
	previousRandRead := randRead
	t.Cleanup(func() { randRead = previousRandRead })
	decryptCount, encryptCount, negativeCount := 0, 0, 0
	for _, vector := range vectors.Decrypt {
		if vector.Suite != "02" {
			continue
		}
		decryptCount++
		if vector.Iter != Iterations {
			t.Fatalf("%s iteration count = %d", vector.Name, vector.Iter)
		}
		got, err := DecryptPassphrase(mustHex(t, vector.Blob), vectorPhrase(t, vector.Name, vector.Passphrase, vector.PassphraseHex))
		plaintext := requiredVectorString(t, vector.Name, "plaintext", vector.Plaintext)
		if err != nil || !bytes.Equal(got, mustHex(t, plaintext)) {
			t.Fatalf("%s: got %x, %v", vector.Name, got, err)
		}
	}
	for _, vector := range vectors.Encrypt {
		if vector.Suite != "02" {
			continue
		}
		encryptCount++
		if vector.Iter != Iterations {
			t.Fatalf("%s iteration count = %d", vector.Name, vector.Iter)
		}
		previous := randRead
		stream := append(mustHex(t, vector.Salt), mustHex(t, vector.IV)...)
		randRead = func(dst []byte) (int, error) { n := copy(dst, stream); stream = stream[n:]; return n, nil }
		plaintext := requiredVectorString(t, vector.Name, "plaintext", vector.Plaintext)
		got := EncryptPassphrase(vectorPhrase(t, vector.Name, vector.Passphrase, vector.PassphraseHex), mustHex(t, plaintext))
		randRead = previous
		if !bytes.Equal(got, mustHex(t, vector.Expected)) {
			t.Fatalf("%s: encrypted blob mismatch", vector.Name)
		}
	}
	for _, vector := range vectors.Negative {
		if vector.Suite != "02" {
			continue
		}
		negativeCount++
		want, known := map[string]error{
			"auth_fail":                ErrAuthFail,
			"reject_truncated":         ErrTruncated,
			"reject_unsupported_suite": ErrUnsupportedSuite,
		}[vector.Expect]
		if !known {
			t.Fatalf("%s: new unsupported expectation %q", vector.Name, vector.Expect)
		}
		blob := requiredVectorString(t, vector.Name, "blob", vector.Blob)
		_, err := DecryptPassphrase(mustHex(t, blob), vectorPhrase(t, vector.Name, vector.Passphrase, vector.PassphraseHex))
		if !errors.Is(err, want) {
			t.Fatalf("%s: error %v, want %v", vector.Name, err, want)
		}
	}
	if decryptCount == 0 || encryptCount == 0 || negativeCount == 0 {
		t.Fatal("suite 02 vector class was empty")
	}
}

func TestEveryCanonicalEncodingVector(t *testing.T) {
	vectors := loadSuite02Vectors(t)
	if len(vectors.Encoding) == 0 || len(vectors.EncodingNegative) == 0 {
		t.Fatal("canonical encoding vector class was empty")
	}
	for _, vector := range vectors.Encoding {
		key := mustHex(t, requiredVectorString(t, vector.Name, "key", vector.Key))
		fragment := requiredVectorString(t, vector.Name, "fragment", vector.Fragment)
		if got := EncodeToBytes(key); !bytes.Equal(got, []byte(fragment)) {
			t.Fatalf("%s: encoded fragment = %q, want %q", vector.Name, got, fragment)
		}
		got, err := DecodeCanonical([]byte(fragment))
		if err != nil || !bytes.Equal(got, key) {
			t.Fatalf("%s: decoded key = %x, %v", vector.Name, got, err)
		}
	}
	for _, vector := range vectors.EncodingNegative {
		if vector.Expect != ErrBadEncoding.Error() {
			t.Fatalf("%s: new unsupported expectation %q", vector.Name, vector.Expect)
		}
		fragment := requiredVectorString(t, vector.Name, "fragment", vector.Fragment)
		if _, err := DecodeCanonical([]byte(fragment)); !errors.Is(err, ErrBadEncoding) {
			t.Fatalf("%s: error %v, want %v", vector.Name, err, ErrBadEncoding)
		}
	}
}
