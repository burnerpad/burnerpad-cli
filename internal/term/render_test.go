package term

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/burnerpad/burnerpad-cli/wordlist"
)

// sixteen real max-length (10-char) list words — the §7.2 worst case
// ("-w 16 phrases certainly do" exceed 80 columns: 16×10+15 = 175 chars).
func maxLenWords(t *testing.T, n int) []string {
	t.Helper()
	var out []string
	for _, w := range wordlist.Words() {
		if len(w) == 10 {
			out = append(out, w)
			if len(out) == n {
				return out
			}
		}
	}
	t.Fatalf("only %d 10-char words", len(out))
	return nil
}

// The §7.2/A10 no-wrap property: the rendered line always fits in strictly
// fewer than width columns (one cell reserved for the cursor), so CR+EL
// repaint on a single physical row can never wrap — at every width 20..120,
// with 16 max-length committed words and every buf/ghost shape.
func TestRenderNeverWraps(t *testing.T) {
	committedSets := [][]string{
		nil,
		{"acrobat"},
		maxLenWords(t, 7),
		maxLenWords(t, 16),
	}
	bufGhost := []struct{ buf, ghost string }{
		{"", ""},
		{"u", "nquenched"}, // 1 typed + 9 ghost (max ghost)
		{"unq", "uenched"}, // mid-word
		{"unquenched", ""}, // full word in buf
		{"yo-", "yo"},      // hyphen path
	}
	for width := 20; width <= 120; width++ {
		for _, cs := range committedSets {
			for _, bg := range bufGhost {
				line := Render(cs, bg.buf, bg.ghost, width)
				if n := utf8.RuneCountInString(line); n >= width {
					t.Fatalf("width %d, %d words, buf=%q ghost=%q: line is %d runes (%q)",
						width, len(cs), bg.buf, bg.ghost, n, line)
				}
				if strings.ContainsAny(line, "\n\r") {
					t.Fatalf("line contains a newline: %q", line)
				}
			}
		}
	}
}

// When everything fits, nothing is elided and the full phrase is visible.
func TestRenderWideShowsEverything(t *testing.T) {
	committed := []string{"acrobat", "cufflink"}
	line := Render(committed, "dre", "sser", 80)
	want := "word 3/7 ▸ acrobat cufflink dresser"
	if line != want {
		t.Fatalf("line = %q, want %q", line, want)
	}
}

// When the committed phrase overflows, the TRAILING portion is shown behind
// a leading `…`, and buf+ghost stay fully visible (the doc's head-elision
// contract): the words remain in the state machine, the counter is
// authoritative.
func TestRenderHeadElision(t *testing.T) {
	committed := maxLenWords(t, 16)
	line := Render(committed, "tul", "ip", 60)
	if utf8.RuneCountInString(line) >= 60 {
		t.Fatalf("overflow: %q", line)
	}
	if !strings.Contains(line, "…") {
		t.Fatalf("expected elision mark: %q", line)
	}
	if !strings.HasSuffix(line, " tulip") {
		t.Fatalf("buf+ghost must stay visible at the end: %q", line)
	}
	// the visible committed tail must be a true suffix of the real phrase
	joined := strings.Join(committed, " ") + " "
	prompt := PromptLabel(len(committed))
	body := strings.TrimPrefix(line, prompt)
	body = strings.TrimSuffix(body, "tulip")
	if i := strings.IndexRune(body, '…'); i < 0 {
		t.Fatalf("no elision in body: %q", body)
	} else if tail := body[i+len("…"):]; !strings.HasSuffix(joined, tail) {
		t.Fatalf("shown committed portion %q is not a suffix of the phrase", tail)
	}
}

// The head-elision formula alone breaks below ~22 columns (prompt 11 + word
// 10 + 1 reserve > 20): A10's stage 2 drops the ghost, stage 3 head-elides
// buf, and the line must still never wrap.
func TestRenderNarrowDegradation(t *testing.T) {
	line := Render(maxLenWords(t, 7), "unquenche", "d", 20)
	if n := utf8.RuneCountInString(line); n >= 20 {
		t.Fatalf("width 20: %d runes: %q", n, line)
	}
	// buf's tail must survive even when the ghost could not
	if !strings.HasSuffix(line, "unquenche") && !strings.Contains(line, "unquenche") {
		t.Fatalf("user's own typed text lost at narrow width: %q", line)
	}
}

// A10 stage 2 in isolation: the ghost loses columns from its end before any
// of the user's own buf is touched.
func TestRenderGhostDroppedBeforeBuf(t *testing.T) {
	// "word 1/7 ▸ " (11) + buf 5 + ghost 5 = 21 > 19 usable: two ghost runes drop.
	pre, gh := renderLine("word 1/7 ▸ ", nil, "quesa", "dilla", 20)
	if pre != "word 1/7 ▸ quesa" {
		t.Fatalf("pre = %q", pre)
	}
	if gh != "dil" {
		t.Fatalf("ghost = %q, want tail-truncated %q", gh, "dil")
	}
}

// §7.2 counter as amended by A11: `word N/7 ▸` while entering words 1..7,
// then the COMMITTED count — `7 words ▸`, never `8 words ▸`.
func TestPromptLabel(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "word 1/7 ▸ "},
		{2, "word 3/7 ▸ "},
		{6, "word 7/7 ▸ "},
		{7, "7 words ▸ "},
		{15, "15 words ▸ "},
	}
	for _, c := range cases {
		if got := PromptLabel(c.n); got != c.want {
			t.Fatalf("PromptLabel(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
