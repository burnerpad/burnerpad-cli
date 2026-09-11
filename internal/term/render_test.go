package term

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/burnerpad/burnerpad-cli/wordlist"
)

// Sixteen real max-length (10-character) list words exceed an 80-column
// terminal: 16*10 + 15 spaces = 175 characters.
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

// The rendered line always fits in strictly fewer than width columns (one
// cell is reserved for the cursor), so CR+EL repaint on a single physical row
// cannot wrap at widths 20..120 with every tested buffer/ghost shape.
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
				pre, ghost := renderLine(promptLabel(len(cs), wordlist.PhraseWords), cs, bg.buf, bg.ghost, width)
				line := pre + ghost
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
	pre, ghost := renderLine(promptLabel(len(committed), wordlist.PhraseWords), committed, "dre", "sser", 80)
	line := pre + ghost
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
	pre, ghost := renderLine(promptLabel(len(committed), wordlist.PhraseWords), committed, "tul", "ip", 60)
	line := pre + ghost
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
	prompt := promptLabel(len(committed), wordlist.PhraseWords)
	body := strings.TrimPrefix(line, prompt)
	body = strings.TrimSuffix(body, "tulip")
	if i := strings.IndexRune(body, '…'); i < 0 {
		t.Fatalf("no elision in body: %q", body)
	} else if tail := body[i+len("…"):]; !strings.HasSuffix(joined, tail) {
		t.Fatalf("shown committed portion %q is not a suffix of the phrase", tail)
	}
}

// The head-elision formula alone breaks below ~22 columns (prompt 11 + word
// 10 + 1 reserve > 20): the second stage drops the ghost, the third head-elides
// the buffer, and the line must still never wrap.
func TestRenderNarrowDegradation(t *testing.T) {
	committed := maxLenWords(t, 7)
	pre, ghost := renderLine(promptLabel(len(committed), wordlist.PhraseWords), committed, "unquenche", "d", 20)
	line := pre + ghost
	if n := utf8.RuneCountInString(line); n >= 20 {
		t.Fatalf("width 20: %d runes: %q", n, line)
	}
	// buf's tail must survive even when the ghost could not
	if !strings.HasSuffix(line, "unquenche") && !strings.Contains(line, "unquenche") {
		t.Fatalf("user's own typed text lost at narrow width: %q", line)
	}
}

// The ghost loses columns from its end before any of the user's own buffer is
// touched.
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

// The counter shows `word N/7 ▸` while entering words 1..7, then the committed
// count: `7 words ▸`, never `8 words ▸`.
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
		if got := promptLabel(c.n, wordlist.PhraseWords); got != c.want {
			t.Fatalf("promptLabel(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
