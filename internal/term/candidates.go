package term

import (
	"sort"
	"strings"
)

// prefixRange returns the half-open index range [lo, hi) of list words that
// have prefix p. Sorted input makes the set contiguous; both bounds are
// binary searches (§7.2: "candidate lookup is a prefix binary search").
func prefixRange(words []string, p string) (int, int) {
	lo := sort.SearchStrings(words, p)
	hi := lo + sort.Search(len(words)-lo, func(i int) bool {
		return !strings.HasPrefix(words[lo+i], p)
	})
	return lo, hi
}

// inList reports exact list membership via binary search.
func inList(words []string, w string) bool {
	i := sort.SearchStrings(words, w)
	return i < len(words) && words[i] == w
}

// inListBytes is inList for a raw byte token. The string(tok) conversions sit
// directly in comparisons, which the compiler performs without allocating —
// so no string copy of a non-list token (possibly foreign secret material,
// §12) is ever made.
func inListBytes(words []string, tok []byte) bool {
	lo, hi := 0, len(words)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if words[mid] < string(tok) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo < len(words) && words[lo] == string(tok)
}

// levenshtein is the plain (non-Damerau) edit distance — the metric §13
// guarantees ≥ 3 pairwise over the list, and the one B24 pins for the
// plain-mode closest-word suggestion.
func levenshtein(a, b string) int {
	if len(a) > len(b) {
		a, b = b, a
	}
	prev := make([]int, len(a)+1)
	cur := make([]int, len(a)+1)
	for i := range prev {
		prev[i] = i
	}
	for j := 1; j <= len(b); j++ {
		cur[0] = j
		for i := 1; i <= len(a); i++ {
			c := 1
			if a[i-1] == b[j-1] {
				c = 0
			}
			m := prev[i-1] + c
			if v := prev[i] + 1; v < m {
				m = v
			}
			if v := cur[i-1] + 1; v < m {
				m = v
			}
			cur[i] = m
		}
		prev, cur = cur, prev
	}
	return prev[len(a)]
}

// closestWord returns the list word nearest to tok by Levenshtein distance
// (first in list order on ties) and that distance. Only distance ≤ 1 is
// guaranteed unambiguous (pairwise list distance ≥ 3, §13/B24).
func closestWord(words []string, tok string) (string, int) {
	best, bestD := "", int(^uint(0)>>1)
	for _, w := range words {
		if d := levenshtein(w, tok); d < bestD {
			best, bestD = w, d
			if d == 0 {
				break
			}
		}
	}
	return best, bestD
}
