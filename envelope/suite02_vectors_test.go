package envelope

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

type suite02VectorFile struct {
	Suites   []string                `json:"suites"`
	Decrypt  []suite02DecryptVector  `json:"decrypt_kat"`
	Encrypt  []suite02EncryptVector  `json:"encrypt_kat"`
	Negative []suite02NegativeVector `json:"negative"`
}

type suite02DecryptVector struct {
	Name          string `json:"name"`
	Suite         string `json:"suite"`
	Passphrase    string `json:"passphrase"`
	PassphraseHex string `json:"passphrase_hex"`
	Blob          string `json:"blob"`
	Plaintext     string `json:"plaintext"`
	Iter          int    `json:"iter"`
}

type suite02EncryptVector struct {
	Name          string `json:"name"`
	Suite         string `json:"suite"`
	Passphrase    string `json:"passphrase"`
	PassphraseHex string `json:"passphrase_hex"`
	Salt          string `json:"salt"`
	IV            string `json:"iv"`
	Plaintext     string `json:"plaintext"`
	Expected      string `json:"expected_blob"`
	Iter          int    `json:"iter"`
}

type suite02NegativeVector struct{ Name, Suite, Passphrase, Blob, Expect string }

func loadSuite02Vectors(t *testing.T) suite02VectorFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/v1.json")
	if err != nil {
		t.Fatal(err)
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

func vectorPhrase(t *testing.T, phrase, encoded string) []byte {
	t.Helper()
	if phrase != "" && encoded != "" {
		t.Fatal("vector has two passphrases")
	}
	if encoded != "" {
		return mustHex(t, encoded)
	}
	return []byte(phrase)
}

func TestEverySuite02Vector(t *testing.T) {
	vectors := loadSuite02Vectors(t)
	decryptCount, encryptCount, negativeCount := 0, 0, 0
	for _, vector := range vectors.Decrypt {
		if vector.Suite != "02" {
			continue
		}
		decryptCount++
		if vector.Iter != Iterations {
			t.Fatalf("%s iteration count = %d", vector.Name, vector.Iter)
		}
		got, err := DecryptPassphrase(mustHex(t, vector.Blob), vectorPhrase(t, vector.Passphrase, vector.PassphraseHex))
		if err != nil || !bytes.Equal(got, mustHex(t, vector.Plaintext)) {
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
		got := EncryptPassphrase(vectorPhrase(t, vector.Passphrase, vector.PassphraseHex), mustHex(t, vector.Plaintext))
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
		_, err := DecryptPassphrase(mustHex(t, vector.Blob), []byte(vector.Passphrase))
		want := map[string]error{"auth_fail": ErrAuthFail, "reject_truncated": ErrTruncated, "reject_unsupported_suite": ErrUnsupportedSuite}[vector.Expect]
		if want == nil {
			t.Fatalf("%s: new unsupported expectation %q", vector.Name, vector.Expect)
		}
		if !errors.Is(err, want) {
			t.Fatalf("%s: error %v, want %v", vector.Name, err, want)
		}
	}
	if decryptCount == 0 || encryptCount == 0 || negativeCount == 0 {
		t.Fatal("suite 02 vector class was empty")
	}
}
