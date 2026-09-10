// Deep-verification tests for phrase generation (white-box, same package):
// a 200k-draw statistical sweep with a per-word chi-square (behind -short),
// domain edges including the full-list draw, and the panic contract at both
// out-of-domain boundaries. Complements wordlist_test.go, which pins the list
// artifact itself and the seam-driven rejection/collision paths.
package wordlist

import (
	"slices"
	"strings"
	"testing"
)

func TestPhraseDomainEdges(t *testing.T) {
	w := Words()
	onList := make(map[string]bool, Count)
	for _, word := range w {
		onList[word] = true
	}

	// n = 1: a single bare list word, no separator.
	p1 := string(phraseN(1))
	if strings.Contains(p1, " ") {
		t.Fatalf("phraseN(1) contains a separator: %q", p1)
	}
	if !onList[p1] {
		t.Fatalf("phraseN(1) = %q is not a list word", p1)
	}

	// n = 16 (the CLI's -w ceiling): 16 distinct on-list words, canonical join.
	p16 := string(phraseN(16))
	parts := strings.Split(p16, " ")
	if len(parts) != 16 {
		t.Fatalf("phraseN(16) has %d tokens: %q", len(parts), p16)
	}
	seen := make(map[string]bool, 16)
	for _, word := range parts {
		if !onList[word] {
			t.Fatalf("off-list word %q in phraseN(16) %q", word, p16)
		}
		if seen[word] {
			t.Fatalf("duplicate word %q in phraseN(16) %q", word, p16)
		}
		seen[word] = true
	}
	if strings.Join(parts, " ") != p16 {
		t.Fatalf("non-canonical join in phraseN(16): %q", p16)
	}
}

// TestPhrasePanicsOutOfDomain pins the [1, Count] domain contract at both
// boundaries (0 and Count+1, plus a negative), asserting the exact panic
// value via recover — the message is part of the extractable package's API.
func TestPhrasePanicsOutOfDomain(t *testing.T) {
	const wantMsg = "wordlist: phrase word count out of range [1, 1296]"
	for _, n := range []int{0, -1, Count + 1} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("phraseN(%d): no panic", n)
					return
				}
				if s, ok := r.(string); !ok || s != wantMsg {
					t.Errorf("phraseN(%d): panic %v, want %q", n, r, wantMsg)
				}
			}()
			phraseN(n)
		}()
	}

	// The in-domain boundaries must NOT panic: 1 and Count are covered by
	// TestPhraseDomainEdges / TestPhraseDeepStatistical respectively.
}

// TestPhraseDeepStatistical draws 200 000 phraseN(7) values through the
// production path (real crypto/rand): every word on-list, 7 distinct words,
// canonical single-space join, and a per-word frequency chi-square over all
// 1296 cells within generous ±6σ bounds. crypto/rand is the real uniformity
// guarantee — this is a tripwire for seam/regression accidents (a scripted
// seam leaking out of a test, a bias slipping into the rejection sampler),
// not an OS-CSPRNG certification. Also exercises the phraseN(Count) full-list
// edge: 1296 words must be exactly a permutation of the list.
func TestPhraseDeepStatistical(t *testing.T) {
	if testing.Short() {
		t.Skip("200k-draw statistical sweep skipped in -short mode")
	}
	w := Words()
	cell := make(map[string]int, Count)
	for i, word := range w {
		cell[word] = i
	}

	const draws = 200_000
	counts := make([]int, Count)
	for i := 0; i < draws; i++ {
		p := string(phraseN(PhraseWords))
		parts := strings.Split(p, " ")
		if len(parts) != PhraseWords {
			t.Fatalf("draw %d: %d tokens in %q", i, len(parts), p)
		}
		var chosen [PhraseWords]int
		for j, word := range parts {
			c, ok := cell[word]
			if !ok {
				t.Fatalf("draw %d: off-list token %q in %q", i, word, p)
			}
			for _, prev := range chosen[:j] {
				if prev == c {
					t.Fatalf("draw %d: duplicate word %q in %q", i, word, p)
				}
			}
			chosen[j] = c
			counts[c]++
		}
		// Canonical join: rebuilding from the split must be byte-identical
		// (catches double spaces and leading/trailing separators).
		if strings.Join(parts, " ") != p {
			t.Fatalf("draw %d: non-canonical join %q", i, p)
		}
	}

	// Per-word chi-square: 1 400 000 word draws over 1296 cells, expected
	// ≈ 1080.25 per cell. 1295 dof → mean 1295, σ = √2590 ≈ 50.9; ±6σ →
	// [989, 1601] (flake odds ≈ 1e-8 — generous sanity, not NIST). The
	// within-phrase distinctness constraint shrinks per-cell variance by a
	// factor (1 − 6/1295) ≈ 0.5%, absorbed by the bound.
	exp := float64(draws*PhraseWords) / float64(Count)
	var chi float64
	minC, maxC := counts[0], counts[0]
	for _, c := range counts {
		d := float64(c) - exp
		chi += d * d / exp
		if c < minC {
			minC = c
		}
		if c > maxC {
			maxC = c
		}
	}
	if chi < 989 || chi > 1601 {
		t.Fatalf("per-word frequency χ² = %.1f outside ±6σ bounds [989, 1601] (1295 dof)", chi)
	}
	// Per-cell ±6σ sanity: exp ± 6·√(exp·(1−1/1296)) ≈ [883, 1278].
	if minC < 883 || maxC > 1278 {
		t.Fatalf("per-word count outside ±6σ cell bounds [883, 1278]: min %d, max %d (exp %.1f)", minC, maxC, exp)
	}

	// Full-domain edge: phraseN(Count) must be a permutation of the whole list.
	full := strings.Split(string(phraseN(Count)), " ")
	if len(full) != Count {
		t.Fatalf("phraseN(%d) has %d tokens", Count, len(full))
	}
	slices.Sort(full)
	for i, word := range full {
		if word != w[i] { // Words() is strictly sorted (pinned invariant)
			t.Fatalf("phraseN(%d) is not a permutation of the list: position %d is %q, want %q", Count, i, word, w[i])
		}
	}

	t.Logf("deep statistical: %d phraseN(7) draws, per-word χ² = %.1f (exp %.2f/cell, min %d, max %d); phraseN(1296) is a full permutation",
		draws, chi, exp, minC, maxC)
}
