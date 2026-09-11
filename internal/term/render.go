package term

import (
	"fmt"
	"strings"
)

const elisionMark = '…'

// promptLabel is the §7.2 counter as amended by A11: the ordinal of the word
// being entered while below the gate (`word 3/7 ▸ `), then the COMMITTED
// count (`7 words ▸ ` — never `8 words ▸`).
func promptLabel(n, min int) string {
	if n < min {
		return fmt.Sprintf("word %d/%d ▸ ", n+1, min)
	}
	return fmt.Sprintf("%d words ▸ ", n)
}

// renderLine builds the line as (everything before the cursor, ghost shown
// after the cursor) so the painter can style the ghost dim and place the
// cursor between them. A10's three-stage degradation:
//  1. elide the committed head as `…` (the words remain in the machine; the
//     counter is authoritative);
//  2. drop ghost columns from the end (the ghost is a hint, buf is the
//     user's own input);
//  3. head-elide the remainder (label+buf too wide) as a last resort.
func renderLine(label string, committed []string, buf, ghost string, width int) (string, string) {
	limit := width - 1 // reserved cursor cell
	if limit < 1 {
		limit = 1 // widths < 20 are --plain territory (§7.2); stay safe anyway
	}
	p := []rune(label)
	b := []rune(buf)
	g := []rune(ghost)

	var ct []rune
	if len(committed) > 0 {
		ct = []rune(strings.Join(committed, " ") + " ")
	}

	budget := limit - len(p) - len(b) - len(g)
	var shown []rune
	switch {
	case budget <= 0 || len(ct) == 0:
		shown = nil
	case len(ct) <= budget:
		shown = ct
	case budget == 1:
		shown = []rune{elisionMark}
	default:
		shown = append([]rune{elisionMark}, ct[len(ct)-(budget-1):]...)
	}

	pre := make([]rune, 0, len(p)+len(shown)+len(b))
	pre = append(pre, p...)
	pre = append(pre, shown...)
	pre = append(pre, b...)

	if over := len(pre) + len(g) - limit; over > 0 {
		drop := min(len(g), over)
		g = g[:len(g)-drop]
		over -= drop
		if over > 0 { // ghost is fully gone here; head-elide what remains
			tail := pre[len(pre)-(limit-1):]
			pre = append([]rune{elisionMark}, tail...)
		}
	}
	return string(pre), string(g)
}
