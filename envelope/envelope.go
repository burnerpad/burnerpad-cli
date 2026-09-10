// Package envelope implements the suite-0x02 subset used by current
// burnerpad-lite clients.
package envelope

import (
	"crypto/aes"
	"crypto/cipher"
)

type Suite byte

const SuitePassphrase Suite = 0x02

const (
	KeyLen   = 32
	IVLen    = 12
	SaltLen  = 16
	TagLen   = 16
	minLen02 = 1 + SaltLen + IVLen + TagLen
)

func aad02(salt, iv []byte) []byte {
	aad := make([]byte, 0, 2+SaltLen+IVLen)
	aad = append(aad, 0x02, 0x02)
	aad = append(aad, salt...)
	return append(aad, iv...)
}

func SuiteOf(blob []byte) (Suite, error) {
	if len(blob) == 0 {
		return 0, ErrTruncated
	}
	if Suite(blob[0]) != SuitePassphrase {
		return 0, ErrUnsupportedSuite
	}
	return SuitePassphrase, nil
}

func EncryptPassphrase(passphrase, plaintext []byte) []byte {
	salt := csprng(SaltLen)
	iv := csprng(IVLen)
	key := deriveKey(passphrase, salt)
	defer wipe(key)
	blob := make([]byte, 0, minLen02+len(plaintext))
	blob = append(blob, byte(SuitePassphrase))
	blob = append(blob, salt...)
	blob = append(blob, iv...)
	return mustGCM(key).Seal(blob, iv, plaintext, aad02(salt, iv))
}

func DecryptPassphrase(blob, passphrase []byte) ([]byte, error) {
	if _, err := SuiteOf(blob); err != nil {
		return nil, err
	}
	if len(blob) < minLen02 {
		return nil, ErrTruncated
	}
	salt := blob[1 : 1+SaltLen]
	iv := blob[1+SaltLen : 1+SaltLen+IVLen]
	key := deriveKey(passphrase, salt)
	defer wipe(key)
	return open(key, iv, blob[1+SaltLen+IVLen:], aad02(salt, iv))
}

func open(key, iv, ciphertext, aad []byte) ([]byte, error) {
	plaintext, err := mustGCM(key).Open(nil, iv, ciphertext, aad)
	if err != nil {
		return nil, ErrAuthFail
	}
	return plaintext, nil
}

func mustGCM(key []byte) cipher.AEAD {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic("invalid AES key length")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic("cannot construct AES-GCM")
	}
	return gcm
}
