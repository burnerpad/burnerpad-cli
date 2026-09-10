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
