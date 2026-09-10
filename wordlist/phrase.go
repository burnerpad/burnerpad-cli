package wordlist

import (
	"crypto/rand"
	"encoding/binary"
)

var randRead = rand.Read // test seam, same pattern and rules as envelope's

// index returns a uniform draw from [0, Count) by rejection-sampling a
// uint16 — no modulo bias. Accept below the largest multiple of 1296 that
// fits in 16 bits (50·1296 = 64800); rejection probability 736/65536 ≈ 1.1%.
// Identical construction to the web driver's randIndex().
func index() uint16 {
	const lim = (65536 / Count) * Count // 64800
	var b [2]byte
	for {
		if _, err := randRead(b[:]); err != nil {
			panic("csprng unavailable: " + err.Error())
		}
		if v := binary.BigEndian.Uint16(b[:]); v < lim {
			return v % Count
		}
	}
}

// Phrase returns a fresh seven-word canonical Burnerpad passphrase. Words are
// distinct, lowercase, and joined by single spaces. The returned bytes belong
// to the caller and can be wiped after use.
func Phrase() []byte { return phraseN(PhraseWords) }

// phraseN retains variable-size generation only as an unexported statistical
// and boundary-test seam. The shipped product has no adjustable word count.
func phraseN(n int) []byte {
	if n < 1 || n > Count {
		panic("wordlist: phrase word count out of range [1, 1296]")
	}
	words := Words()
	idx := make([]uint16, 0, n)
draw:
	for len(idx) < n {
		v := index()
		for _, prev := range idx {
			if prev == v {
				continue draw // distinctness: redraw on collision (matches web)
			}
		}
		idx = append(idx, v)
	}
	size := n - 1 // the joining spaces
	for _, v := range idx {
		size += len(words[v])
	}
	out := make([]byte, 0, size) // exact-size alloc: no growth reallocs leaving stale copies
	for i, v := range idx {
		if i > 0 {
			out = append(out, ' ')
		}
		out = append(out, words[v]...)
	}
	for i := range idx {
		idx[i] = 0
	}
	return out
}
