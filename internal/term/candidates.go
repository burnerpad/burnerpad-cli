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
