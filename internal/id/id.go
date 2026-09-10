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

// Normalize mirrors the current server's public identifier normalization:
// Unicode upcase → strip only "-" → fold I,L→1 and O→0 → require exactly
// 26 alphabet bytes. The length check applies after normalization.
//
// Known fail-closed divergence: Elixir's String.upcase applies full case
// mapping (ß→SS, ﬁ→FI) where strings.ToUpper applies only simple per-rune
// mapping and leaves those runes unchanged; the unchanged rune then fails
// the alphabet check. Go therefore rejects a handful of exotic inputs the
// server would accept — never the reverse.
func Normalize(raw string) (string, error) {
	up := strings.ToUpper(raw)
	// Strip/fold byte-wise: '-', 'I', 'L', 'O' are ASCII and cannot occur
	// inside a UTF-8 multibyte sequence, so this matches the server's
	// grapheme-wise String.replace calls.
	var b strings.Builder
	b.Grow(len(up))
	for i := 0; i < len(up); i++ {
		c := up[i]
		switch c {
		case '-':
			continue
		case 'I', 'L':
			c = '1'
		case 'O':
			c = '0'
		}
		b.WriteByte(c)
	}
	norm := b.String()
	if len(norm) != 26 {
		return "", ErrBadID
	}
	for i := 0; i < len(norm); i++ {
		if strings.IndexByte(alphabet, norm[i]) < 0 {
			return "", ErrBadID
		}
	}
	return norm, nil
}
