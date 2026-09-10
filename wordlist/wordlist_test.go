// The §13 invariants gates + phrase generation tests. White-box (same
// package): the seam tests swap the unexported randRead.
package wordlist

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

const (
	// Pinned in §13: the embedded file (LF-separated, one trailing LF)…
	fileSHA256 = "7aa57a4d3ecf6581729992bad9575bacdebf7c28378af2aec6a50f11aec326f5"
	// …and the canonical space-joined form both repos can assert.
	joinedSHA256 = "dc3267b1a27f952a27f4a3f70dcb097ff7a3c84a229ac2f88b055a2a07923134"
)

func TestWordlistInvariants(t *testing.T) {
	// Embedded bytes are exactly the frozen artifact.
	sum := sha256.Sum256([]byte(raw))
	if got := hex.EncodeToString(sum[:]); got != fileSHA256 {
		t.Fatalf("embedded wordlist drifted: sha256 = %s, pinned %s", got, fileSHA256)
	}
	if !strings.HasSuffix(raw, "\n") || strings.HasSuffix(raw, "\n\n") {
		t.Fatal("wordlist must end with exactly one trailing LF")
	}

	w := Words()
	if len(w) != Count {
		t.Fatalf("count = %d, want %d", len(w), Count)
	}

	// Strict order ⇒ sorted AND distinct in one check (§13).
	for i := 1; i < len(w); i++ {
		if w[i-1] >= w[i] {
			t.Fatalf("not strictly sorted at %d: %q >= %q", i, w[i-1], w[i])
		}
	}

	// Unique 3-char prefix — the autocomplete's load-bearing property —
	// plus length and charset bounds.
	prefixes := make(map[string]string, Count)
	for _, word := range w {
		if len(word) < 3 {
			t.Fatalf("word %q shorter than the 3-char prefix property requires", word)
		}
		if len(word) > 10 {
			t.Fatalf("word %q longer than 10", word)
		}
		for _, c := range []byte(word) {
			if (c < 'a' || c > 'z') && c != '-' {
				t.Fatalf("word %q outside charset [a-z-]", word)
			}
		}
		p := word[:3]
		if prev, dup := prefixes[p]; dup {
			t.Fatalf("3-char prefix collision: %q vs %q", prev, word)
		}
		prefixes[p] = word
	}

	// Prefix-freeness (review amendment B23): no word is a prefix of another.
	// Space/Enter's "candidates == 1" commit rule depends on this; it follows
	// from unique-at-3 only for words ≥ 3 chars, so it is asserted directly —
	// a future short-word edit must fail here, not silently break commits.
	// Sorted order makes the adjacent-pair check sufficient.
	for i := 1; i < len(w); i++ {
		if strings.HasPrefix(w[i], w[i-1]) {
			t.Fatalf("prefix-freeness violated: %q is a prefix of %q", w[i-1], w[i])
		}
	}

	// Canonical space-joined form pinned — the cross-repo drift tripwire.
	jsum := sha256.Sum256([]byte(strings.Join(w, " ")))
	if got := hex.EncodeToString(jsum[:]); got != joinedSHA256 {
		t.Fatalf("space-joined form drifted: sha256 = %s, pinned %s", got, joinedSHA256)
	}
}

// TestWordlistEditDistance verifies the spoken-channel property §13 pins:
// pairwise plain Levenshtein distance ≥ 3, and the minimum is exactly 3.
// O(n²); skipped under -short.
func TestWordlistEditDistance(t *testing.T) {
	if testing.Short() {
		t.Skip("O(n²) edit-distance sweep skipped in -short mode")
	}
	w := Words()
	min := 1 << 30
	var minA, minB string
	var row, prev [11 + 1]int
	for i := 0; i < len(w); i++ {
		for j := i + 1; j < len(w); j++ {
			a, b := w[i], w[j]
			for k := 0; k <= len(b); k++ {
				prev[k] = k
			}
			for x := 1; x <= len(a); x++ {
				row[0] = x
				for y := 1; y <= len(b); y++ {
					cost := 1
					if a[x-1] == b[y-1] {
						cost = 0
					}
					m := prev[y] + 1 // deletion
					if v := row[y-1] + 1; v < m {
						m = v // insertion
					}
					if v := prev[y-1] + cost; v < m {
						m = v // substitution
					}
					row[y] = m
				}
				prev, row = row, prev
			}
			if d := prev[len(b)]; d < min {
				min, minA, minB = d, a, b
			}
		}
	}
	if min < 3 {
		t.Fatalf("Levenshtein bound broken: d(%q,%q) = %d < 3", minA, minB, min)
	}
	if min != 3 {
		t.Fatalf("expected the minimum pairwise distance to be exactly 3, got %d", min)
	}
}

// scriptRand swaps the seam for a scripted stream — same pattern and rules as
// envelope's fixRand (§14.1): draw order and exact consumption are asserted.
func scriptRand(t *testing.T, script ...[]byte) {
	t.Helper()
	stream := bytes.Join(script, nil)
	prevFn := randRead
	randRead = func(b []byte) (int, error) {
		if len(stream) < len(b) {
			t.Fatalf("scripted randomness over-consumed")
		}
		copy(b, stream[:len(b)])
		stream = stream[len(b):]
		return len(b), nil
	}
	t.Cleanup(func() {
		randRead = prevFn
		if len(stream) != 0 {
			t.Errorf("scripted randomness not fully consumed (%d bytes left)", len(stream))
		}
	})
}

// TestPhraseSeamRejectionAndCollision drives both loops deterministically
// (§13): a rejection-sampled redraw (v ≥ 64800) and a distinctness redraw
// (duplicate index), including the exact acceptance boundary.
func TestPhraseSeamRejectionAndCollision(t *testing.T) {
	w := Words()

	t.Run("rejection-boundary", func(t *testing.T) {
		// 0xFD20 = 64800 → rejected (v < lim strictly); 0xFD1F = 64799 →
		// accepted, 64799 % 1296 = 1295 (the last word).
		scriptRand(t,
			[]byte{0xFD, 0x20}, // == lim → reject
			[]byte{0xFF, 0xFF}, // 65535 → reject
			[]byte{0xFD, 0x1F}, // 64799 → accept → index 1295
		)
		got := phraseN(1)
		if string(got) != w[1295] {
			t.Fatalf("phraseN(1) = %q, want %q", got, w[1295])
		}
	})

	t.Run("collision-redraw", func(t *testing.T) {
		scriptRand(t,
			[]byte{0x00, 0x05}, // index 5
			[]byte{0x00, 0x05}, // duplicate → distinctness redraw
			[]byte{0x01, 0x00}, // index 256
		)
		got := phraseN(2)
		want := w[5] + " " + w[256]
		if string(got) != want {
			t.Fatalf("phraseN(2) = %q, want %q", got, want)
		}
	})
}

// TestPhraseStatistical is the §13 smoke test: 10k phrases — every word
// on-list, n distinct words each, canonical single-space join, no byte
// outside [a-z -].
func TestPhraseStatistical(t *testing.T) {
	w := Words()
	onList := make(map[string]bool, Count)
	for _, word := range w {
		onList[word] = true
	}
	for i := 0; i < 10000; i++ {
		n := 7
		if i%10 == 0 {
			n = 1 + i%16 // sweep 1..16 on every tenth draw
		}
		p := phraseN(n)
		for _, c := range p {
			if (c < 'a' || c > 'z') && c != '-' && c != ' ' {
				t.Fatalf("byte %q outside [a-z -] in %q", c, p)
			}
		}
		parts := strings.Split(string(p), " ")
		if len(parts) != n {
			t.Fatalf("phraseN(%d) has %d words: %q", n, len(parts), p)
		}
		seen := make(map[string]bool, n)
		for _, word := range parts {
			if !onList[word] {
				t.Fatalf("off-list word %q in %q", word, p)
			}
			if seen[word] {
				t.Fatalf("duplicate word %q in %q", word, p)
			}
			seen[word] = true
		}
		// Canonical join: rebuilding from the split must be byte-identical
		// (no double spaces, no leading/trailing space).
		if rejoined := strings.Join(parts, " "); rejoined != string(p) {
			t.Fatalf("non-canonical join: %q", p)
		}
	}
}
