package term

import (
	"fmt"
	"strings"

	"github.com/burnerpad/burnerpad-cli/wordlist"
)

// machineOutput is the machine's complete post-event surface: everything a
// renderer needs, nothing about how to draw it. phrase is populated (fresh
// bytes the caller owns and wipes) only once done is true.
type machineOutput struct {
	committed []string // validated, distinct words (copy — safe to keep)
	buf       string   // current partial list word
	ghost     string   // unique-completion remainder; "" unless exactly 1 candidate
	status    string   // one-line status-zone content ("" = clear)
	bell      bool     // ring the terminal bell
	done      bool     // a complete phrase was accepted → attempt decrypt
	phrase    []byte   // canonical phrase bytes when done; caller wipes
}

// machine is the pure autocomplete state machine: no I/O and no terminal
// knowledge. State is exactly the committed words, current buffer, submission
// gate, and completion state.
type machine struct {
	words     []string
	minWords  int
	committed []string
	buf       []rune
	done      bool
}

// newMachine returns a machine over the embedded word list. min is the
// committed-word gate for submission; min <= 0 selects
// wordlist.PhraseWords.
func newMachine(min int) *machine {
	if min <= 0 {
		min = wordlist.PhraseWords
	}
	return &machine{words: wordlist.Words(), minWords: min}
}

// committedCopy returns a copy of the committed words.
func (m *machine) committedCopy() []string { return append([]string(nil), m.committed...) }

// phraseBytes builds the canonical phrase as fresh bytes the caller wipes.
func (m *machine) phraseBytes() []byte {
	var out []byte
	for i, w := range m.committed {
		if i > 0 {
			out = append(out, ' ')
		}
		out = append(out, w...)
	}
	return out
}

// --- candidate machinery -------------------------------------------------

// committedWithPrefix counts committed words having prefix p. Committed words
// are always list words, so they fall inside prefixRange(p) when prefixed.
func (m *machine) committedWithPrefix(p string) int {
	n := 0
	for _, w := range m.committed {
		if strings.HasPrefix(w, p) {
			n++
		}
	}
	return n
}

func (m *machine) isCommitted(w string) bool {
	for _, c := range m.committed {
		if c == w {
			return true
		}
	}
	return false
}

// candCount is the number of list words with prefix p, excluding words that
// are already committed.
func (m *machine) candCount(p string) int {
	lo, hi := prefixRange(m.words, p)
	return (hi - lo) - m.committedWithPrefix(p)
}

// firstCandidates returns up to k candidates in list order.
func (m *machine) firstCandidates(p string, k int) []string {
	lo, hi := prefixRange(m.words, p)
	out := make([]string, 0, k)
	for i := lo; i < hi && len(out) < k; i++ {
		if !m.isCommitted(m.words[i]) {
			out = append(out, m.words[i])
		}
	}
	return out
}

// lcpOfCandidates returns the longest common prefix of the candidate set
// (which necessarily extends p). Empty candidate set → p unchanged.
func (m *machine) lcpOfCandidates(p string) string {
	cs := m.firstCandidates(p, m.candCount(p))
	if len(cs) == 0 {
		return p
	}
	lcp := cs[0]
	for _, c := range cs[1:] {
		j := 0
		for j < len(lcp) && j < len(c) && lcp[j] == c[j] {
			j++
		}
		lcp = lcp[:j]
	}
	return lcp
}

// ghost returns the unique-completion remainder for the current buf.
func (m *machine) ghost() string {
	if len(m.buf) == 0 {
		return ""
	}
	p := string(m.buf)
	if m.candCount(p) != 1 {
		return ""
	}
	c := m.firstCandidates(p, 1)
	return strings.TrimPrefix(c[0], p)
}

// --- status strings ------------------------------------------------------
//
// Count-vs-example thresholds are pinned by transcript tests ("126 words
// match", "10 match: academy accountant acetone …").

const (
	submitHint            = "Enter submits — keep typing if the phrase was longer"
	rejectedCharacterHint = "character rejected: no available Burnerpad word matches"
)

func (m *machine) matchStatus() string {
	if len(m.buf) == 0 {
		return ""
	}
	n := m.candCount(string(m.buf))
	switch {
	case n <= 1:
		return "" // the ghost (or the reject that just fired) is the signal
	case n > 10:
		return fmt.Sprintf("%d words match", n)
	default:
		ex := m.firstCandidates(string(m.buf), 3)
		s := fmt.Sprintf("%d match: %s", n, strings.Join(ex, " "))
		if n > 3 {
			s += " …"
		}
		return s
	}
}

func (m *machine) ambiguousStatus() string {
	n := m.candCount(string(m.buf))
	ex := m.firstCandidates(string(m.buf), 5)
	s := "still ambiguous: " + strings.Join(ex, " ")
	if n > 5 {
		s += " …"
	}
	return s
}

// needMoreStatus is the below-gate Enter status.
func (m *machine) needMoreStatus() string {
	return fmt.Sprintf("%d/%d — need at least %d words",
		len(m.committed), m.minWords, m.minWords)
}

// commit appends w, clears buf, and returns a status using the committed-word
// count.
func (m *machine) commit(w string) (bool, string) {
	if len(m.committed) >= wordlist.MaxPhraseWords {
		return true, interactiveWordsMessage(interactiveWordsTooMany)
	}
	m.committed = append(m.committed, w)
	m.buf = m.buf[:0]
	if n := len(m.committed); n >= m.minWords {
		return false, fmt.Sprintf("%d words · %s", n, submitHint)
	}
	return false, ""
}

// --- event handling ------------------------------------------------------

// handle applies one event and returns the complete resulting surface.
// kindCtrlC, kindCtrlD, and kindIgnored are no-ops here: interruption and
// EOF are flow control, owned by the prompt loop, not phrase state.
func (m *machine) handle(e event) machineOutput {
	var bell bool
	var status string
	switch e.Kind {
	case kindRune:
		bell, status = m.onRune(e.R)
	case kindSpace:
		bell, status = m.onSpace()
	case kindTab:
		bell, status = m.onTab()
	case kindEnter:
		bell, status = m.onEnter()
	case kindBackspace:
		bell, status = m.onBackspace()
	case kindCtrlW:
		if len(m.buf) > 0 {
			m.buf = m.buf[:0]
		} else if n := len(m.committed); n > 0 {
			// "if already empty, delete the last committed word entirely"
			m.committed = m.committed[:n-1]
		}
	case kindCtrlU:
		m.buf = m.buf[:0] // "clear buf (committed words untouched)"
	case kindCtrlO:
		bell, status = m.onCtrlO()
	case kindPaste:
		bell, status = m.onPaste(e.Paste)
	case kindInputTooLong:
		bell, status = true, "paste rejected: input is too long"
	}
	out := machineOutput{
		committed: m.committedCopy(),
		buf:       string(m.buf),
		ghost:     m.ghost(),
		status:    status,
		bell:      bell,
		done:      m.done,
	}
	if m.done {
		out.phrase = m.phraseBytes()
	}
	return out
}

func (m *machine) onRune(r rune) (bool, string) {
	if r < ' ' || r == 0x7f {
		return false, "" // control bytes never reach the buffer
	}
	if r >= 'A' && r <= 'Z' {
		r += 'a' - 'A' // ASCII uppercase is lowercased silently
	}
	// Accept any printable rune iff at least one candidate would remain. The list
	// charset is [a-z-], so digits/foreign punctuation still always reject —
	// and '-' is typeable exactly where the list needs it (yo-yo).
	try := string(m.buf) + string(r)
	if m.candCount(try) >= 1 {
		m.buf = append(m.buf, r)
		return false, m.matchStatus()
	}
	return true, rejectedCharacterHint
}

func (m *machine) onSpace() (bool, string) {
	if len(m.buf) == 0 {
		return false, "" // "Empty buf → ignored"
	}
	p := string(m.buf)
	if m.candCount(p) == 1 {
		return m.commit(m.firstCandidates(p, 1)[0])
	}
	return true, m.ambiguousStatus()
}

func (m *machine) onTab() (bool, string) {
	if len(m.buf) == 0 {
		return false, ""
	}
	p := string(m.buf)
	// Tab ACCEPTS a unique candidate — the ghosted word is committed and buf
	// clears, so the next word can be typed immediately (the browser gesture:
	// Tab takes the suggestion and puts you after the space). This is still
	// no auto-commit (ADR-0012): the ghost has shown the whole word first,
	// and Tab is as deliberate a keystroke as Space.
	if m.candCount(p) == 1 {
		return m.commit(m.firstCandidates(p, 1)[0])
	}
	// For an ambiguous buffer, extend to the longest common prefix. The LCP is
	// a prefix of every candidate, so this branch never changes the candidate
	// count — it cannot make the buffer unique behind the user's back.
	m.buf = []rune(m.lcpOfCandidates(p))
	return false, m.matchStatus()
}

// onEnter commits only when the buffer is non-empty and submits only when the
// buffer is empty with at least min committed words. Commit and submit never
// share a keystroke, so the gesture that gates the claim request is deliberate.
func (m *machine) onEnter() (bool, string) {
	if len(m.buf) > 0 {
		p := string(m.buf)
		if m.candCount(p) != 1 {
			return true, m.ambiguousStatus() // "Ambiguous buf → bell"
		}
		bell, status := m.commit(m.firstCandidates(p, 1)[0])
		if bell {
			return true, status
		}
		if len(m.committed) < m.minWords {
			status = m.needMoreStatus()
		}
		return false, status
	}
	if len(m.committed) >= m.minWords {
		m.done = true
		return false, ""
	}
	return false, m.needMoreStatus()
}

func (m *machine) onBackspace() (bool, string) {
	if len(m.buf) > 0 {
		m.buf = m.buf[:len(m.buf)-1]
		return false, m.matchStatus()
	}
	if n := len(m.committed); n > 0 {
		// "un-commit the previous word back into buf as editable text"
		w := m.committed[n-1]
		m.committed = m.committed[:n-1]
		m.buf = []rune(w)
		return false, m.matchStatus()
	}
	return false, ""
}

// onCtrlO refuses the retired free-form escape without changing state.
func (m *machine) onCtrlO() (bool, string) {
	return true, "every passphrase word must be on the Burnerpad word list"
}

// onPaste applies the atomic whole-phrase rule: ASCII-lowercase, split on
// ASCII whitespace runs, every token in the list, distinct, and not already
// committed. It is all-or-nothing, leaves a partial buffer untouched, and
// never submits.
func (m *machine) onPaste(text []byte) (bool, string) {
	tokens, issue := parseInteractiveWords(text, m.committed)
	if issue != interactiveWordsOK {
		return true, "paste rejected: " + interactiveWordsMessage(issue)
	}
	if len(tokens) == 0 {
		return false, ""
	}
	keep := m.buf // paste leaves a partial buffer untouched; commit clears it
	var status string
	for _, tok := range tokens {
		_, status = m.commit(tok)
	}
	m.buf = keep
	return false, status
}
