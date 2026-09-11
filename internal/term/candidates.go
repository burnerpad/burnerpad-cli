package term

import (
	"sort"
	"strings"
)

// prefixRange returns the half-open index range [lo, hi) of list words that
// have prefix p. Sorted input makes the set contiguous, so both bounds use
// binary search.
func prefixRange(words []string, p string) (int, int) {
	lo := sort.SearchStrings(words, p)
	hi := lo + sort.Search(len(words)-lo, func(i int) bool {
		return !strings.HasPrefix(words[lo+i], p)
	})
	return lo, hi
}
