package wordlist

import (
	_ "embed"
	"strings"
	"sync"
)

// EFF Short Wordlist #2 (eff_short_wordlist_2_0), © Electronic Frontier
// Foundation, licensed CC BY 3.0 — https://www.eff.org/dice. Embedded data
// only, indices stripped, otherwise unmodified. Attribution ships in NOTICE
// and via `burnerpad licenses` (a go:embed of NOTICE — the `go install` path
// yields a bare binary, so the embedded copy is the attribution that always
// ships). The list is FROZEN: any edit is a breaking change to phrase-entry
// UX and must fail TestWordlistInvariants.
//
//go:embed eff_short_wordlist_2_0.txt
var raw string

const (
	Count       = 1296 // 6^4; ~10.34 bits/word
	PhraseWords = 7    // 7·log2(1296) ≈ 72.4 bits

	// FileSHA256 pins the embedded LF-separated list; surfaced by
	// `burnerpad version` (§25) and independently asserted by
	// TestWordlistInvariants against the embedded bytes.
	FileSHA256 = "7aa57a4d3ecf6581729992bad9575bacdebf7c28378af2aec6a50f11aec326f5"
)

// Words returns the sorted list. Sortedness gives the autocomplete's prefix
// binary search for free — no index structure needed.
var Words = sync.OnceValue(func() []string {
	w := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	if len(w) != Count {
		panic("wordlist corrupt") // unreachable if TestWordlistInvariants passes
	}
	return w
})
