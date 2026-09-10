package term

import (
	"fmt"
	"strings"

	"github.com/burnerpad/burnerpad-cli/wordlist"
)

// Mode identifies the only supported passphrase-entry language.
type Mode int

const (
	ListLocked Mode = iota // ghost-text autocomplete over the 1296-word list
)

// Output is the machine's complete post-event surface: everything a renderer
// needs, nothing about how to draw it. Phrase is populated (fresh bytes the
// caller owns and wipes) only once Done is true.
type Output struct {
	Committed []string // validated, distinct words (copy — safe to keep)
	Buf       string   // current partial list word
	Ghost     string   // unique-completion remainder; "" unless exactly 1 candidate
	Status    string   // one-line status-zone content ("" = clear)
	Bell      bool     // ring the terminal bell
	Done      bool     // a complete phrase was accepted → attempt decrypt
	Phrase    []byte   // canonical phrase bytes when Done; caller wipes
}

// Machine is the pure autocomplete state machine of §7.2 (as amended by
// A9–A12) — no I/O, no terminal knowledge. State is exactly: committed
// []word + buf + mode (§7.2 "State:").
type Machine struct {
	words     []string
	minWords  int
	committed []string
	buf       []rune
	done      bool
	seeded    bool // at the untouched §7.3 seeded start; first gesture clears it
}

// NewMachine returns a machine over the embedded word list. min is the
// committed-word gate for submission (§7.2: ≥ 7 for burnerpad phrases);
// min ≤ 0 selects wordlist.PhraseWords.
func NewMachine(min int) *Machine {
	if min <= 0 {
		min = wordlist.PhraseWords
	}
	return &Machine{words: wordlist.Words(), minWords: min}
}

// NewMintingMachine returns the canonical list-locked phrase machine.
func NewMintingMachine(min int) *Machine {
	return NewMachine(min)
}

// Committed returns a copy of the committed words.
func (m *Machine) Committed() []string { return append([]string(nil), m.committed...) }

// seedStatus is the §7.3 kept-words status line, verbatim from the doc's
// auth-fail screen.
const seedStatus = "(your words are kept; Backspace steps into them)"

// seedable reports whether seed is a committed list that min-gated
// list-locked entry could itself have produced: at least min words, each on
// the list, all distinct. Anything else is not a §7.3 retained state.
func seedable(words, seed []string, min int) bool {
	if len(seed) < min || len(seed) > wordlist.MaxPhraseWords {
		return false
	}
	seen := make(map[string]bool, len(seed))
	for _, w := range seed {
		if !inList(words, w) || seen[w] {
			return false
		}
		seen[w] = true
	}
	return true
}

// Seed pre-commits words — the §7.3 wrong-passphrase retry state ("The
// committed words are retained for editing"). It takes only on a fresh
// machine (nothing committed, empty buf, list-locked, not done) and only a
// seed that min-gated entry could itself have produced (seedable); the seeded
// machine then behaves EXACTLY as if the words had just been committed:
// Backspace un-commits the last one into buf, Enter on the empty buf submits,
// candidate exclusion and the renderer see ordinary committed words. The
// returned Output is the initial §7.3 surface (kept-words status). ok=false
// leaves the machine untouched — the caller prompts fresh.
func (m *Machine) Seed(words []string) (out Output, ok bool) {
	if m.done || len(m.committed) > 0 || len(m.buf) > 0 {
		return Output{}, false
	}
	if !seedable(m.words, words, m.minWords) {
		return Output{}, false
	}
	m.committed = append([]string(nil), words...)
	m.seeded = true
	return Output{Committed: m.Committed(), Status: seedStatus}, true
}

// SeededIntact reports whether the machine still sits at the untouched
// seeded start — the one surface where the §7.3 label (`word 7/7 ▸`) and
// kept-words status apply. The first gesture ends it; the A11 labels resume.
func (m *Machine) SeededIntact() bool { return m.seeded }

// SeedWords parses a previously submitted canonical phrase back into the
// committed word list it was built from — the §7.3 retry seed. min is the
// committed-word gate the next prompt will run with (≤ 0 selects
// wordlist.PhraseWords). The words come back only when the phrase is exactly
// what min-gated list-locked entry submits: ≥ min distinct list words joined
// by single spaces. Anything else — free-form phrases above all — returns
// nil, and no string copy of a non-qualifying phrase is ever made: tokens
// are checked against the list as raw bytes first, so string material is
// minted only for public list words (only the SELECTION is secret, and the
// Machine holds committed words as strings anyway).
func SeedWords(phrase []byte, min int) []string {
	if min <= 0 {
		min = wordlist.PhraseWords
	}
	words := wordlist.Words()
	// Pass 1, bytes only: single-space framing and per-token list membership.
	var starts, ends []int
	start := 0
	for i := 0; i <= len(phrase); i++ {
		if i < len(phrase) && phrase[i] != ' ' {
			continue
		}
		if i == start {
			return nil // empty token: leading/trailing/double space, or empty phrase
		}
		if !inListBytes(words, phrase[start:i]) {
			return nil
		}
		starts, ends = append(starts, start), append(ends, i)
		if len(starts) > wordlist.MaxPhraseWords {
			return nil
		}
		start = i + 1
	}
	if len(starts) < min {
		return nil
	}
	// Pass 2: every token is a public list word — strings are safe to mint.
	out := make([]string, len(starts))
	seen := make(map[string]bool, len(starts))
	for i := range starts {
		w := string(phrase[starts[i]:ends[i]])
		if seen[w] {
			return nil // duplicates: not producible by list-locked entry
		}
		seen[w] = true
		out[i] = w
	}
	return out
}

// Buf returns the current partial word.
func (m *Machine) Buf() string { return string(m.buf) }

// Mode returns the current entry mode.
func (m *Machine) Mode() Mode { return ListLocked }

// Done reports whether a complete phrase has been accepted.
func (m *Machine) Done() bool { return m.done }

// phraseBytes builds the canonical phrase as fresh bytes the caller wipes.
func (m *Machine) phraseBytes() []byte {
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
func (m *Machine) committedWithPrefix(p string) int {
	n := 0
	for _, w := range m.committed {
		if strings.HasPrefix(w, p) {
			n++
		}
	}
	return n
}

func (m *Machine) isCommitted(w string) bool {
	for _, c := range m.committed {
		if c == w {
			return true
		}
	}
	return false
}

// candCount is |{list words with prefix p}| minus already-committed ones
// (§7.2 "Candidates = list words with prefix buf, excluding already-committed
// words").
func (m *Machine) candCount(p string) int {
	lo, hi := prefixRange(m.words, p)
	return (hi - lo) - m.committedWithPrefix(p)
}

// firstCandidates returns up to k candidates in list order.
func (m *Machine) firstCandidates(p string, k int) []string {
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
func (m *Machine) lcpOfCandidates(p string) string {
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
func (m *Machine) ghost() string {
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
// Count-vs-example thresholds are pinned by the §10(c) transcript
// ("126 words match", "10 match: academy accountant acetone …"); the
// exclusion-empty and toggle-hint rules by B24 and the §7.2 Ctrl+O row.

const decryptHint = "Enter decrypts — keep typing if the phrase was longer"

func (m *Machine) matchStatus() string {
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

func (m *Machine) ambiguousStatus() string {
	n := m.candCount(string(m.buf))
	ex := m.firstCandidates(string(m.buf), 5)
	s := "still ambiguous: " + strings.Join(ex, " ")
	if n > 5 {
		s += " …"
	}
	return s
}

// needMoreStatus is the below-gate Enter status.
func (m *Machine) needMoreStatus() string {
	return fmt.Sprintf("%d/%d — need at least %d words",
		len(m.committed), m.minWords, m.minWords)
}

// commit appends w, clears buf, and returns the post-commit status
// (committed-count form, A11).
func (m *Machine) commit(w string) (bool, string) {
	if len(m.committed) >= wordlist.MaxPhraseWords {
		return true, interactiveWordsMessage(interactiveWordsTooMany)
	}
	m.committed = append(m.committed, w)
	m.buf = m.buf[:0]
	if n := len(m.committed); n >= m.minWords {
		return false, fmt.Sprintf("%d words · %s", n, decryptHint)
	}
	return false, ""
}

// --- event handling ------------------------------------------------------

// Handle applies one event and returns the complete resulting surface.
// KindCtrlC, KindCtrlD, and KindIgnored are no-ops here: interruption and
// EOF are flow control, owned by the prompt loop, not phrase state.
func (m *Machine) Handle(e Event) Output {
	switch e.Kind {
	case KindCtrlC, KindCtrlD, KindIgnored:
		// flow control: the §7.3 seeded start survives them like any state
	default:
		m.seeded = false // any real gesture ends the seeded-start surface
	}
	var bell bool
	var status string
	switch e.Kind {
	case KindRune:
		bell, status = m.onRune(e.R)
	case KindSpace:
		bell, status = m.onSpace()
	case KindTab:
		bell, status = m.onTab()
	case KindEnter:
		bell, status = m.onEnter()
	case KindBackspace:
		bell, status = m.onBackspace()
	case KindCtrlW:
		if len(m.buf) > 0 {
			m.buf = m.buf[:0]
		} else if n := len(m.committed); n > 0 {
			// "if already empty, delete the last committed word entirely"
			m.committed = m.committed[:n-1]
		}
	case KindCtrlU:
		m.buf = m.buf[:0] // "clear buf (committed words untouched)"
	case KindCtrlO:
		bell, status = m.onCtrlO()
	case KindPaste:
		bell, status = m.onPaste(e.Paste)
	}
	out := Output{
		Committed: m.Committed(),
		Buf:       string(m.buf),
		Ghost:     m.ghost(),
		Status:    status,
		Bell:      bell,
		Done:      m.done,
	}
	if m.done {
		out.Phrase = m.phraseBytes()
	}
	return out
}

func (m *Machine) onRune(r rune) (bool, string) {
	if r < ' ' || r == 0x7f {
		return false, "" // control bytes never reach the buffer
	}
	if r >= 'A' && r <= 'Z' {
		r += 'a' - 'A' // ASCII uppercase is lowercased silently
	}
	// A9: accept any printable rune iff ≥ 1 candidate would remain. The list
	// charset is [a-z-], so digits/foreign punctuation still always reject —
	// and '-' is typeable exactly where the list needs it (yo-yo).
	try := string(m.buf) + string(r)
	if m.candCount(try) >= 1 {
		m.buf = append(m.buf, r)
		return false, m.matchStatus()
	}
	// B24: with committed words excluded, candidates can be 0 while list
	// words DO start with try (type "app" after committing "apple") — the
	// "no word starts with" claim would be false there; distinguish it.
	if lo, hi := prefixRange(m.words, try); hi-lo > 0 {
		return true, fmt.Sprintf("only already-committed words start with %q", try)
	}
	return true, fmt.Sprintf("no word starts with %q", try)
}

func (m *Machine) onSpace() (bool, string) {
	if len(m.buf) == 0 {
		return false, "" // "Empty buf → ignored"
	}
	p := string(m.buf)
	if m.candCount(p) == 1 {
		return m.commit(m.firstCandidates(p, 1)[0])
	}
	return true, m.ambiguousStatus()
}

func (m *Machine) onTab() (bool, string) {
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
	// Ambiguous: A12's "extends buf to the longest common prefix". The LCP is
	// a prefix of every candidate, so this branch never changes the candidate
	// count — it cannot make the buffer unique behind the user's back.
	m.buf = []rune(m.lcpOfCandidates(p))
	return false, m.matchStatus()
}

// onEnter implements A12: Enter with a non-empty buf COMMITS ONLY; Enter
// with an empty buf and committed ≥ min SUBMITS. Commit-and-submit never
// share a keystroke — the gesture that gates the claim request is always
// deliberate.
func (m *Machine) onEnter() (bool, string) {
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

func (m *Machine) onBackspace() (bool, string) {
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
func (m *Machine) onCtrlO() (bool, string) {
	return true, "every passphrase word must be on the Burnerpad word list"
}

// onPaste implements the atomic whole-phrase rule (§7.2 paste row, B24):
// ASCII-lowercase, split on ASCII whitespace runs, every token ∈ list ∧ distinct ∧ not
// already committed; all-or-nothing; a partial buf is left untouched; paste
// never submits.
func (m *Machine) onPaste(text []byte) (bool, string) {
	tokens, issue := parseInteractiveWords(text, m.committed)
	if issue != interactiveWordsOK {
		return true, "paste rejected: " + interactiveWordsMessage(issue)
	}
	if len(tokens) == 0 {
		return false, ""
	}
	keep := m.buf // B24: paste leaves a partial buf untouched (commit clears it)
	var status string
	for _, tok := range tokens {
		_, status = m.commit(tok)
	}
	m.buf = keep
	return false, status
}
