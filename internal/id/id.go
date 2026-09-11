// Package id implements current 26-character Crockford normalization and
// operation-specific full-share-URL parsing.
package id

import (
	"errors"
	"strings"
)

// Sentinel errors — the package's entire error surface. Both are constant
// category text: a rejected target may still be a sensitive share capability,
// so no error ever echoes any part of the input.
var (
	// ErrBadID: the id is not valid Crockford base32 after normalization.
	ErrBadID = errors.New("invalid id")
	// ErrBadTarget is a content-free target rejection.
	ErrBadTarget = errors.New("invalid share target")
)

// alphabet is the server's Crockford base32 alphabet (store.ex @alphabet):
// digits plus uppercase letters without I, L, O, U.
const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Normalize mirrors the current server's public identifier normalization for
// ASCII input: uppercase → strip only "-" → fold I,L→1 and O→0 → require
// exactly 26 alphabet bytes. Non-ASCII input is rejected before case folding.
func Normalize(raw string) (string, error) {
	var normalized [26]byte
	n := 0
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c >= 0x80 {
			return "", ErrBadID
		}
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		switch c {
		case '-':
			continue
		case 'I', 'L':
			c = '1'
		case 'O':
			c = '0'
		}
		if n == len(normalized) || strings.IndexByte(alphabet, c) < 0 {
			return "", ErrBadID
		}
		normalized[n] = c
		n++
	}
	if n != len(normalized) {
		return "", ErrBadID
	}
	return string(normalized[:]), nil
}
