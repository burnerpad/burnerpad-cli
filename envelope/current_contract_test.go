package envelope

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
)

func mustHex(t *testing.T, value string) []byte {
	t.Helper()
	b, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSuite02CurrentBrowserVector(t *testing.T) {
	phrase := []byte("correct horse battery staple")
	blob := mustHex(t, "02000102030405060708090a0b0c0d0e0f000102030405060708090a0b7eb4e7e504d748e58347a707fe9df8dc96eaca44ac2ebd1456734951f204d7a78bd1e97a9c8cf9c68b8ce783763476")
	want := []byte("production DB password: hunter2")
	got, err := DecryptPassphrase(blob, phrase)
	if err != nil {
		t.Fatalf("DecryptPassphrase: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("plaintext = %q", got)
	}
	if _, err := DecryptPassphrase(blob, []byte("wrong phrase")); !errors.Is(err, ErrAuthFail) {
		t.Fatalf("wrong phrase error = %v", err)
	}
}

func TestSuite02EncryptMatchesBrowserVector(t *testing.T) {
	previous := randRead
	stream := mustHex(t, "000102030405060708090a0b0c0d0e0f000102030405060708090a0b")
	randRead = func(dst []byte) (int, error) {
		n := copy(dst, stream)
		stream = stream[n:]
		return n, nil
	}
	t.Cleanup(func() { randRead = previous })
	got := EncryptPassphrase([]byte("correct horse battery staple"), []byte("production DB password: hunter2"))
	want := mustHex(t, "02000102030405060708090a0b0c0d0e0f000102030405060708090a0b7eb4e7e504d748e58347a707fe9df8dc96eaca44ac2ebd1456734951f204d7a78bd1e97a9c8cf9c68b8ce783763476")
	if !bytes.Equal(got, want) {
		t.Fatalf("blob mismatch\n got %x\nwant %x", got, want)
	}
}

func TestOnlySuite02IsSupported(t *testing.T) {
	if _, err := SuiteOf([]byte{0x01}); !errors.Is(err, ErrUnsupportedSuite) {
		t.Fatalf("suite 0x01 error = %v", err)
	}
	if _, err := SuiteOf([]byte{0x03}); !errors.Is(err, ErrUnsupportedSuite) {
		t.Fatalf("suite 0x03 error = %v", err)
	}
}

func TestCanonicalBase64URL(t *testing.T) {
	for _, bad := range []string{"A=", "A+", "A/", "AA\n", "AB"} {
		if _, err := DecodeCanonical([]byte(bad)); !errors.Is(err, ErrBadEncoding) {
			t.Errorf("DecodeCanonical(%q) = %v", bad, err)
		}
	}
	raw := []byte{0, 1, 2, 253, 254, 255}
	encoded := EncodeToBytes(raw)
	decoded, err := DecodeCanonical(encoded)
	if err != nil || !bytes.Equal(decoded, raw) {
		t.Fatalf("round trip = %x, %v", decoded, err)
	}
}
