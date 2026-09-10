package term

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/burnerpad/burnerpad-cli/wordlist"
)

// --- tiny step DSL for table-driven event tests (ported from the prototype) --

func rn(r rune) Event      { return Event{Kind: KindRune, R: r} }
func kd(k EventKind) Event { return Event{Kind: k} }
func paste(s string) Event { return Event{Kind: KindPaste, Paste: []byte(s)} }

type expect struct {
	buf       *string
	ghost     *string
	bell      *bool
	status    *string
	committed []string // nil = don't check
	done      *bool
}

type step struct {
	ev Event
	expect
}

func sp(s string) *string { return &s }
func bp(b bool) *bool     { return &b }

func typeRunes(s string) []step {
	var out []step
	for _, r := range s {
		out = append(out, step{ev: rn(r)})
	}
	return out
}

func runSteps(t *testing.T, m *Machine, steps []step) {
	t.Helper()
	for i, st := range steps {
		out := m.Handle(st.ev)
		if st.buf != nil && out.Buf != *st.buf {
			t.Fatalf("step %d (%+v): buf = %q, want %q", i, st.ev, out.Buf, *st.buf)
		}
		if st.ghost != nil && out.Ghost != *st.ghost {
			t.Fatalf("step %d (%+v): ghost = %q, want %q", i, st.ev, out.Ghost, *st.ghost)
		}
		if st.bell != nil && out.Bell != *st.bell {
			t.Fatalf("step %d (%+v): bell = %v, want %v (status %q)", i, st.ev, out.Bell, *st.bell, out.Status)
		}
		if st.status != nil && out.Status != *st.status {
			t.Fatalf("step %d (%+v): status = %q, want %q", i, st.ev, out.Status, *st.status)
		}
		if st.committed != nil && !slices.Equal(out.Committed, st.committed) {
			t.Fatalf("step %d (%+v): committed = %v, want %v", i, st.ev, out.Committed, st.committed)
		}
		if st.done != nil && out.Done != *st.done {
			t.Fatalf("step %d (%+v): done = %v, want %v", i, st.ev, out.Done, *st.done)
		}
	}
}

const ctrlOHint = ""

// --- §7.2 key table, row by row (A9–A12, B3, B24) --------------------------

func TestKeyTable(t *testing.T) {
	cases := []struct {
		name  string
		steps []step
	}{
		{
			name: "printable a-z appends iff >=1 candidate remains",
			steps: []step{
				{ev: rn('a'), expect: expect{buf: sp("a"), bell: bp(false)}},
				{ev: rn('c'), expect: expect{buf: sp("ac"), bell: bp(false)}},
				{ev: rn('s'), expect: expect{ // "acs" matches nothing
					buf: sp("ac"), bell: bp(true), status: sp(`no word starts with "acs"`)}},
			},
		},
		{
			name: "uppercase lowercased silently",
			steps: append(typeRunes("ACR"),
				step{ev: kd(KindSpace), expect: expect{committed: []string{"acrobat"}, buf: sp("")}}),
		},
		{
			name: "uppercase folds before candidate check (ghost appears)",
			steps: []step{
				{ev: rn('A'), expect: expect{buf: sp("a")}},
				{ev: rn('C'), expect: expect{buf: sp("ac")}},
				{ev: rn('R'), expect: expect{buf: sp("acr"), ghost: sp("obat")}},
			},
		},
		{
			name: "digits rejected in list-locked mode",
			steps: append(typeRunes("wa"),
				step{ev: rn('7'), expect: expect{buf: sp("wa"), bell: bp(true),
					status: sp(`no word starts with "wa7"`)}}),
		},
		{
			name: "punctuation (non-hyphen) rejected in list-locked mode",
			steps: append(typeRunes("wa"),
				step{ev: rn('!'), expect: expect{buf: sp("wa"), bell: bp(true),
					status: sp(`no word starts with "wa!"`)}}),
		},
		{
			name: "candidates==1 shows remainder as ghost",
			steps: append(typeRunes("tu"),
				step{ev: rn('l'), expect: expect{buf: sp("tul"), ghost: sp("ip")}}),
		},
		{
			name: "Space commits unique word; counter advances via committed",
			steps: append(typeRunes("acr"),
				step{ev: kd(KindSpace), expect: expect{committed: []string{"acrobat"}, buf: sp(""), ghost: sp(""), bell: bp(false)}}),
		},
		{
			name: "Space on ambiguous buf bells with first<=5 candidates",
			steps: append(typeRunes("ap"),
				// prefix "ap": apartment apnea apostrophe apple apricot — exactly 5, no ellipsis
				step{ev: kd(KindSpace), expect: expect{bell: bp(true), buf: sp("ap"),
					status: sp("still ambiguous: apartment apnea apostrophe apple apricot")}}),
		},
		{
			name: "Space on ambiguous buf with >5 candidates gets ellipsis",
			steps: append(typeRunes("a"),
				step{ev: kd(KindSpace), expect: expect{bell: bp(true),
					status: sp("still ambiguous: aardvark abandoned abbreviate abdomen abhorrence …")}}),
		},
		{
			name: "Space on empty buf ignored",
			steps: []step{
				{ev: kd(KindSpace), expect: expect{buf: sp(""), bell: bp(false), status: sp(""), committed: []string{}}},
			},
		},
		{
			name: "Tab extends to longest common prefix (q -> qu, still 4 candidates)",
			steps: []step{
				{ev: rn('q'), expect: expect{buf: sp("q")}},
				{ev: kd(KindTab), expect: expect{buf: sp("qu"), committed: []string{},
					status: sp("4 match: quarters quesadilla quilt …")}},
			},
		},
		{
			name: "Tab accepts a unique candidate: commits it and clears buf for the next word",
			steps: append(typeRunes("aq"), // aquamarine is unique at "aq"
				step{ev: kd(KindTab), expect: expect{committed: []string{"aquamarine"}, buf: sp(""),
					ghost: sp(""), bell: bp(false), status: sp("")}},
				// buf is empty, so the next rune starts word 2 with no Space needed
				step{ev: rn('t'), expect: expect{committed: []string{"aquamarine"}, buf: sp("t")}}),
		},
		{
			name: "Tab-committed word excludes itself from later candidates (like Space)",
			steps: append(typeRunes("aq"),
				step{ev: kd(KindTab), expect: expect{committed: []string{"aquamarine"}}},
				step{ev: rn('a'), expect: expect{buf: sp("a")}},
				step{ev: rn('q'), expect: expect{buf: sp("a"), bell: bp(true),
					status: sp(`only already-committed words start with "aq"`)}}),
		},
		{
			name: "Tab at the gate reports the same commit status Space does",
			steps: append([]step{{ev: paste("acrobat blender cufflink dishcloth eggnog fondue")}},
				append(typeRunes("tul"),
					step{ev: kd(KindTab), expect: expect{buf: sp(""), committed: []string{"acrobat",
						"blender", "cufflink", "dishcloth", "eggnog", "fondue", "tulip"},
						status: sp("7 words · " + submitHint)}})...),
		},
		{
			name: "Tab on empty buf is a no-op",
			steps: []step{
				{ev: kd(KindTab), expect: expect{buf: sp(""), bell: bp(false), committed: []string{}}},
			},
		},
		{
			name: "Enter commits unique buf ONLY; below 7 words reports the count (A12+B3)",
			steps: append(typeRunes("acr"),
				step{ev: kd(KindEnter), expect: expect{committed: []string{"acrobat"}, buf: sp(""),
					done: bp(false), status: sp("1/7 — need at least 7 words" + ctrlOHint)}}),
		},
		{
			name: "Enter with empty buf below 7 words: status, no bell, no done",
			steps: []step{
				{ev: kd(KindEnter), expect: expect{done: bp(false), bell: bp(false),
					status: sp("0/7 — need at least 7 words" + ctrlOHint)}},
			},
		},
		{
			name: "Enter on ambiguous buf bells",
			steps: append(typeRunes("ap"),
				step{ev: kd(KindEnter), expect: expect{bell: bp(true), done: bp(false), buf: sp("ap")}}),
		},
		{
			name: "Backspace deletes last char of buf",
			steps: append(typeRunes("tup"),
				step{ev: kd(KindBackspace), expect: expect{buf: sp("tu"), ghost: sp("")}}),
		},
		{
			name: "Backspace on empty buf un-commits previous word into buf",
			steps: append(typeRunes("acr"),
				step{ev: kd(KindSpace), expect: expect{committed: []string{"acrobat"}}},
				step{ev: kd(KindBackspace), expect: expect{committed: []string{}, buf: sp("acrobat")}},
				// the un-committed word is editable text: shave it down and go elsewhere
				step{ev: kd(KindBackspace), expect: expect{buf: sp("acroba")}},
			),
		},
		{
			name: "Backspace on empty buf with nothing committed is a no-op",
			steps: []step{
				{ev: kd(KindBackspace), expect: expect{buf: sp(""), bell: bp(false), committed: []string{}}},
			},
		},
		{
			name: "Ctrl+W clears buf; committed untouched",
			steps: append(typeRunes("acr"),
				step{ev: kd(KindCtrlW), expect: expect{buf: sp(""), committed: []string{}}}),
		},
		{
			name: "Ctrl+W on empty buf deletes last committed word entirely",
			steps: []step{
				{ev: paste("acrobat cufflink"), expect: expect{committed: []string{"acrobat", "cufflink"}}},
				{ev: kd(KindCtrlW), expect: expect{committed: []string{"acrobat"}, buf: sp("")}},
			},
		},
		{
			name: "Ctrl+U clears buf only",
			steps: []step{
				{ev: paste("acrobat")},
				{ev: rn('t'), expect: expect{buf: sp("t")}},
				{ev: kd(KindCtrlU), expect: expect{buf: sp(""), committed: []string{"acrobat"}}},
			},
		},
		{
			name: "paste: all tokens valid commits all",
			steps: []step{
				{ev: paste("acrobat cufflink dresser"), expect: expect{
					committed: []string{"acrobat", "cufflink", "dresser"}, buf: sp(""), bell: bp(false)}},
			},
		},
		{
			name: "paste: lowercased and split on whitespace runs",
			steps: []step{
				{ev: paste("  Acrobat\tCUFFLINK \n dresser  "), expect: expect{
					committed: []string{"acrobat", "cufflink", "dresser"}}},
			},
		},
		{
			name: "paste: any invalid token rejects the whole paste",
			steps: []step{
				{ev: paste("acrobat osmoss dresser"), expect: expect{
					committed: []string{}, bell: bp(true),
					status: sp("paste rejected: a word is not on the Burnerpad word list")}},
			},
		},
		{
			name: "paste: duplicate within paste rejects whole",
			steps: []step{
				{ev: paste("acrobat acrobat"), expect: expect{
					committed: []string{}, bell: bp(true),
					status: sp("paste rejected: a word is repeated")}},
			},
		},
		{
			name: "paste: token already committed rejects whole",
			steps: []step{
				{ev: paste("acrobat")},
				{ev: paste("cufflink acrobat"), expect: expect{
					committed: []string{"acrobat"}, bell: bp(true),
					status: sp("paste rejected: a word is repeated")}},
			},
		},
		{
			name: "paste: never partially applied even when prefix of tokens is valid",
			steps: []step{
				{ev: paste("acrobat cufflink dresser osmosis riverboat tulip zzzz"), expect: expect{
					committed: []string{}, bell: bp(true)}},
			},
		},
		{
			name: "paste: empty/whitespace-only is a no-op",
			steps: []step{
				{ev: paste("  \n\t "), expect: expect{committed: []string{}, bell: bp(false), status: sp("")}},
			},
		},
		{
			name: "paste: leaves a partial buf untouched (B24)",
			steps: append(typeRunes("tu"),
				step{ev: paste("acrobat"), expect: expect{committed: []string{"acrobat"}, buf: sp("tu")}}),
		},
		{
			name: "Ctrl+O cannot leave list-locked entry",
			steps: []step{
				{ev: kd(KindCtrlO), expect: expect{buf: sp(""), bell: bp(true),
					status: sp("every passphrase word must be on the Burnerpad word list")}},
			},
		},
		{
			name: "rejections never advertise a free-form escape",
			steps: append(typeRunes("wa"),
				step{ev: rn('x'), expect: expect{bell: bp(true), status: sp(`no word starts with "wax"`)}},
				step{ev: rn('x'), expect: expect{bell: bp(true),
					status: sp(`no word starts with "wax"`)}}),
		},
		{
			name: "committed words are excluded from candidates",
			steps: []step{
				{ev: paste("apple")},
				// "ap" now has 4 candidates, not 5
				{ev: rn('a')}, {ev: rn('p'), expect: expect{
					status: sp("4 match: apartment apnea apostrophe …")}},
				// "app" only matched apple, which is committed → reject, honest message (B24)
				{ev: rn('p'), expect: expect{buf: sp("ap"), bell: bp(true),
					status: sp(`only already-committed words start with "app"`)}},
			},
		},
		{
			name: "3-char word: unique at keystroke 3 with EMPTY ghost, Space commits",
			steps: append(typeRunes("cup"),
				step{ev: kd(KindSpace), expect: expect{committed: []string{"cup"}, buf: sp("")}}),
		},
		{
			name: "Ctrl+C, Ctrl+D and ignored events are state no-ops",
			steps: append(typeRunes("acr"),
				step{ev: kd(KindCtrlC), expect: expect{buf: sp("acr"), ghost: sp("obat"), bell: bp(false)}},
				step{ev: kd(KindCtrlD), expect: expect{buf: sp("acr")}},
				step{ev: kd(KindIgnored), expect: expect{buf: sp("acr"), committed: []string{}}}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runSteps(t, NewMachine(0), tc.steps)
		})
	}
}

// A12: Enter with a non-empty buf commits ONLY — even the 7th word. The
// submit is always a second, deliberate Enter on an empty buf. (The
// prototype allowed commit+submit in one keystroke; amendment A12 overrode
// that, and this test pins the amended rule.)
func TestEnterCommitsSeventhWordThenSecondEnterSubmits(t *testing.T) {
	m := NewMachine(0)
	out := m.Handle(paste("acrobat cufflink dresser osmosis riverboat tulip"))
	if len(out.Committed) != 6 {
		t.Fatalf("setup: committed %v", out.Committed)
	}
	runSteps(t, m, typeRunes("wol"))
	out = m.Handle(kd(KindEnter))
	if out.Done {
		t.Fatal("A12: Enter with non-empty buf must commit only, never submit")
	}
	if want := "7 words · Enter submits — keep typing if the phrase was longer"; out.Status != want {
		t.Fatalf("commit status = %q, want %q", out.Status, want)
	}
	out = m.Handle(kd(KindEnter))
	if !out.Done {
		t.Fatal("second Enter with empty buf at 7 words: done = false")
	}
	want := "acrobat cufflink dresser osmosis riverboat tulip wolverine"
	if got := string(out.Phrase); got != want {
		t.Fatalf("phrase = %q, want %q", got, want)
	}
}

func TestSubmissionHintIsOperationNeutral(t *testing.T) {
	for _, command := range []string{"create", "reveal", "decrypt", "burn"} {
		if strings.Contains(strings.ToLower(submitHint), command) {
			t.Fatalf("shared submission hint names %q: %q", command, submitHint)
		}
	}
}

// Enter gating: exactly at 7 committed + empty buf → done; at 6 → status.
func TestEnterGating(t *testing.T) {
	m := NewMachine(0)
	m.Handle(paste("acrobat cufflink dresser osmosis riverboat tulip"))
	out := m.Handle(kd(KindEnter))
	if out.Done || out.Status != "6/7 — need at least 7 words"+ctrlOHint {
		t.Fatalf("at 6 words: done=%v status=%q", out.Done, out.Status)
	}
	m.Handle(paste("wolverine"))
	out = m.Handle(kd(KindEnter))
	if !out.Done {
		t.Fatalf("at 7 words + empty buf: done = false")
	}
}

// An 8th word can be committed and Enter still submits (phrases longer than
// 7 are legal; only ≥7 is guaranteed).
func TestEighthWordThenEnter(t *testing.T) {
	m := NewMachine(0)
	m.Handle(paste("acrobat cufflink dresser osmosis riverboat tulip wolverine"))
	runSteps(t, m, typeRunes("zeb"))
	out := m.Handle(kd(KindSpace))
	if out.Status != "8 words · Enter submits — keep typing if the phrase was longer" {
		t.Fatalf("8th commit status = %q", out.Status)
	}
	if out.Done {
		t.Fatal("Space must never trigger decrypt")
	}
	out = m.Handle(kd(KindEnter))
	if !out.Done || string(out.Phrase) != "acrobat cufflink dresser osmosis riverboat tulip wolverine zebra" {
		t.Fatalf("done=%v phrase=%q", out.Done, out.Phrase)
	}
}

// NewMachine's gate is parameterized for focused terminal tests.
func TestCustomMinGate(t *testing.T) {
	m := NewMachine(3)
	m.Handle(paste("acrobat cufflink"))
	out := m.Handle(kd(KindEnter))
	if out.Done || out.Status != "2/3 — need at least 3 words"+ctrlOHint {
		t.Fatalf("at 2/3: done=%v status=%q", out.Done, out.Status)
	}
	out = m.Handle(paste("dresser"))
	if want := "3 words · Enter submits — keep typing if the phrase was longer"; out.Status != want {
		t.Fatalf("3rd commit status = %q", out.Status)
	}
	out = m.Handle(kd(KindEnter))
	if !out.Done || string(out.Phrase) != "acrobat cufflink dresser" {
		t.Fatalf("done=%v phrase=%q", out.Done, out.Phrase)
	}
}

// Output.Committed must be a defensive copy.
func TestOutputCommittedIsACopy(t *testing.T) {
	m := NewMachine(0)
	out := m.Handle(paste("acrobat cufflink"))
	out.Committed[0] = "corrupted"
	if got := m.Committed()[0]; got != "acrobat" {
		t.Fatalf("machine state aliased by Output.Committed: %q", got)
	}
}

// Tab's two branches, over every reachable 1-, 2- and 3-char prefix: a
// unique candidate is COMMITTED (buf clears, ready for the next word), and
// an ambiguous one only extends buf to the LCP — which, being a prefix of
// every candidate, can never change the candidate count. Together: Tab never
// commits a word the ghost had not already shown in full.
func TestTabCommitsUniqueElseHoldsCandidateCount(t *testing.T) {
	words := wordlist.Words()
	prefixes := map[string]bool{}
	for _, w := range words {
		for n := 1; n <= 3 && n <= len(w); n++ {
			prefixes[w[:n]] = true
		}
	}
	var unique, ambiguous int
	for p := range prefixes {
		m := NewMachine(0)
		for _, r := range p {
			m.Handle(rn(r))
		}
		before := m.candCount(m.Buf())
		want := m.firstCandidates(m.Buf(), 1)
		out := m.Handle(kd(KindTab))
		if before == 1 {
			unique++
			if out.Buf != "" || !slices.Equal(out.Committed, want) {
				t.Fatalf("prefix %q: Tab on a unique candidate gave committed=%v buf=%q, want %v and an empty buf",
					p, out.Committed, out.Buf, want)
			}
			continue
		}
		ambiguous++
		if len(out.Committed) != 0 {
			t.Fatalf("prefix %q: Tab committed %v with %d candidates", p, out.Committed, before)
		}
		if after := m.candCount(out.Buf); before != after {
			t.Fatalf("prefix %q: Tab changed candidate count %d → %d", p, before, after)
		}
	}
	if unique == 0 || ambiguous == 0 {
		t.Fatalf("both Tab branches must be exercised: unique=%d ambiguous=%d", unique, ambiguous)
	}
}

// The un-commit → edit → recommit loop: wrong-word correction without
// retyping the phrase (§7.2 Backspace row rationale).
func TestUncommitEditRecommit(t *testing.T) {
	m := NewMachine(0)
	m.Handle(paste("acrobat tupperware"))
	m.Handle(kd(KindBackspace)) // "tupperware" back into buf
	for range len("tupperware") - len("tu") {
		m.Handle(kd(KindBackspace))
	}
	out := m.Handle(rn('l'))
	if out.Buf != "tul" || out.Ghost != "ip" {
		t.Fatalf("buf=%q ghost=%q", out.Buf, out.Ghost)
	}
	out = m.Handle(kd(KindSpace))
	want := []string{"acrobat", "tulip"}
	if !reflect.DeepEqual(out.Committed, want) {
		t.Fatalf("committed = %v, want %v", out.Committed, want)
	}
}

// Sanity: after every event of a long random-ish walk, a non-empty buf in
// list-locked mode always has ≥1 candidate (the machine's core invariant).
func TestBufAlwaysHasCandidates(t *testing.T) {
	m := NewMachine(0)
	events := []Event{
		rn('a'), rn('c'), rn('r'), kd(KindSpace), rn('z'), kd(KindTab), rn('9'),
		kd(KindBackspace), kd(KindBackspace), rn('q'), kd(KindTab), kd(KindCtrlW), kd(KindBackspace),
		rn('x'), kd(KindEnter), kd(KindCtrlU), paste("tulip"), kd(KindBackspace), kd(KindSpace),
	}
	for i, e := range events {
		m.Handle(e)
		if m.Mode() == ListLocked && m.Buf() != "" && m.candCount(m.Buf()) < 1 {
			t.Fatalf("after event %d (%+v): buf %q has no candidates", i, e, m.Buf())
		}
	}
}

// --- §7.3 seeded retry ------------------------------------------------------

// seedWords7 is the doc's canonical 7-word phrase as a committed list.
var seedWords7 = []string{"acrobat", "cufflink", "dresser", "osmosis", "riverboat", "tulip", "wolverine"}

// The §7.3 auth-fail screen, byte for byte: a seeded start surfaces the kept
// words with the `word 7/7 ▸` label (not A11's `7 words ▸`) and the verbatim
// kept-words status.
func TestSeededStartIsTheSection73Screen(t *testing.T) {
	m := NewMachine(0)
	out, ok := m.Seed(seedWords7)
	if !ok {
		t.Fatal("Seed rejected a valid 7-word list selection")
	}
	if !slices.Equal(out.Committed, seedWords7) || out.Buf != "" || out.Ghost != "" || out.Bell || out.Done {
		t.Fatalf("seeded surface = %+v", out)
	}
	if out.Status != "(your words are kept; Backspace steps into them)" {
		t.Fatalf("seeded status = %q", out.Status)
	}
	if !m.SeededIntact() {
		t.Fatal("SeededIntact = false at the seeded start")
	}
	// §7.3 pins the seeded-start label form.
	if got := seededLabel(len(out.Committed)); got != "word 7/7 ▸ " {
		t.Fatalf("seeded label = %q, want %q", got, "word 7/7 ▸ ")
	}
	// The seeded words render as ordinary committed words on the prompt row.
	pre, gh := renderLine(seededLabel(7), out.Committed, out.Buf, out.Ghost, 80)
	if want := "word 7/7 ▸ acrobat cufflink dresser osmosis riverboat tulip wolverine "; pre != want || gh != "" {
		t.Fatalf("seeded render = %q + ghost %q, want %q", pre, gh, want)
	}
}

// Enter at the seeded start resubmits the kept phrase unchanged — the seeded
// state IS a committed-7-words state.
func TestSeededEnterResubmits(t *testing.T) {
	m := NewMachine(0)
	if _, ok := m.Seed(seedWords7); !ok {
		t.Fatal("seed rejected")
	}
	out := m.Handle(kd(KindEnter))
	if !out.Done || string(out.Phrase) != strings.Join(seedWords7, " ") {
		t.Fatalf("done=%v phrase=%q", out.Done, out.Phrase)
	}
}

// The §7.3 rationale end to end: one wrong word, corrected by stepping into
// the kept words with Backspace instead of retyping seven.
func TestSeededBackspaceStepsIntoWords(t *testing.T) {
	wrong := []string{"acrobat", "cufflink", "dresser", "osmosis", "riverboat", "tulip", "zebra"}
	m := NewMachine(0)
	if _, ok := m.Seed(wrong); !ok {
		t.Fatal("seed rejected")
	}
	out := m.Handle(kd(KindBackspace))
	if out.Buf != "zebra" || !slices.Equal(out.Committed, wrong[:6]) {
		t.Fatalf("Backspace into seed: buf=%q committed=%v", out.Buf, out.Committed)
	}
	if m.SeededIntact() {
		t.Fatal("a gesture must end the seeded start")
	}
	m.Handle(kd(KindCtrlU)) // clear the wrong word, keep the six
	runSteps(t, m, typeRunes("wol"))
	out = m.Handle(kd(KindSpace))
	if !slices.Equal(out.Committed, seedWords7) {
		t.Fatalf("corrected committed = %v", out.Committed)
	}
	out = m.Handle(kd(KindEnter))
	if !out.Done || string(out.Phrase) != strings.Join(seedWords7, " ") {
		t.Fatalf("done=%v phrase=%q", out.Done, out.Phrase)
	}
}

// Seed + fresh typing mix: the kept words accept an appended 8th word exactly
// like ordinarily committed ones (phrases longer than the gate are legal).
func TestSeededPlusFreshTyping(t *testing.T) {
	m := NewMachine(0)
	if _, ok := m.Seed(seedWords7); !ok {
		t.Fatal("seed rejected")
	}
	runSteps(t, m, typeRunes("zeb"))
	if m.SeededIntact() {
		t.Fatal("typing must end the seeded start")
	}
	out := m.Handle(kd(KindSpace))
	if len(out.Committed) != 8 || out.Status != "8 words · Enter submits — keep typing if the phrase was longer" {
		t.Fatalf("8th commit: committed=%v status=%q", out.Committed, out.Status)
	}
	out = m.Handle(kd(KindEnter))
	if want := strings.Join(seedWords7, " ") + " zebra"; !out.Done || string(out.Phrase) != want {
		t.Fatalf("done=%v phrase=%q", out.Done, out.Phrase)
	}
}

// Seeded words are ordinary committed words for candidate exclusion too
// (B24's honest reject variant).
func TestSeededWordsExcludedFromCandidates(t *testing.T) {
	seed := []string{"apple", "acrobat", "cufflink", "dresser", "osmosis", "riverboat", "tulip"}
	m := NewMachine(0)
	if _, ok := m.Seed(seed); !ok {
		t.Fatal("seed rejected")
	}
	m.Handle(rn('a'))
	m.Handle(rn('p'))
	out := m.Handle(rn('p'))
	if !out.Bell || out.Status != `only already-committed words start with "app"` {
		t.Fatalf("bell=%v status=%q", out.Bell, out.Status)
	}
}

// Flow-control events do not end the seeded start (Ctrl+C ownership lives in
// the prompt loop; a resize repaint must keep the §7.3 label).
func TestSeededSurvivesFlowControlEvents(t *testing.T) {
	m := NewMachine(0)
	if _, ok := m.Seed(seedWords7); !ok {
		t.Fatal("seed rejected")
	}
	m.Handle(kd(KindCtrlC))
	m.Handle(kd(KindCtrlD))
	m.Handle(kd(KindIgnored))
	if !m.SeededIntact() {
		t.Fatal("flow-control events must not end the seeded start")
	}
}

// Seed accepts only what min-gated list-locked entry could have produced, and
// only on a fresh machine; every rejection leaves the machine untouched.
func TestSeedRejections(t *testing.T) {
	reject := func(name string, m *Machine, seed []string) {
		t.Helper()
		before := m.Committed()
		if _, ok := m.Seed(seed); ok {
			t.Fatalf("%s: seed accepted", name)
		}
		if !slices.Equal(m.Committed(), before) || m.SeededIntact() {
			t.Fatalf("%s: rejected seed mutated the machine", name)
		}
	}
	reject("non-list word", NewMachine(0),
		[]string{"acrobat", "cufflink", "dresser", "osmosis", "riverboat", "tulip", "zzznotaword"})
	reject("duplicate word", NewMachine(0),
		[]string{"acrobat", "acrobat", "dresser", "osmosis", "riverboat", "tulip", "wolverine"})
	reject("below the gate", NewMachine(0), seedWords7[:6])
	reject("above the maximum", NewMachine(0), wordlist.Words()[:wordlist.MaxPhraseWords+1])
	reject("empty seed", NewMachine(0), nil)

	dirty := NewMachine(0)
	dirty.Handle(paste("acrobat"))
	reject("non-fresh machine", dirty, seedWords7)

	// A custom gate flows through: 2 list words seed a min-2 machine.
	m := NewMachine(2)
	if _, ok := m.Seed([]string{"cup", "elk"}); !ok {
		t.Fatal("min-2 seed rejected")
	}
	if out := m.Handle(kd(KindEnter)); !out.Done || string(out.Phrase) != "cup elk" {
		t.Fatalf("min-2 resubmit: %+v", out)
	}
}

// SeedWords is the inverse of the canonical phrase join — and ONLY that: it
// yields the committed list for exactly what min-gated list-locked entry
// submits, nil for everything else.
func TestSeedWords(t *testing.T) {
	phrase := "acrobat cufflink dresser osmosis riverboat tulip wolverine"
	if got := SeedWords([]byte(phrase), 0); !slices.Equal(got, seedWords7) {
		t.Fatalf("SeedWords(%q) = %v", phrase, got)
	}
	for name, p := range map[string]string{
		"free-form":         "S3cret Pa$$ phrase!",
		"non-list word":     "acrobat cufflink dresser osmosis riverboat tulip zzznotaword",
		"uppercase":         "Acrobat cufflink dresser osmosis riverboat tulip wolverine",
		"double space":      "acrobat  cufflink dresser osmosis riverboat tulip wolverine",
		"trailing space":    "acrobat cufflink dresser osmosis riverboat tulip wolverine ",
		"leading space":     " acrobat cufflink dresser osmosis riverboat tulip wolverine",
		"duplicate word":    "acrobat acrobat dresser osmosis riverboat tulip wolverine",
		"below the gate":    "acrobat cufflink dresser osmosis riverboat tulip",
		"empty":             "",
		"single word":       "acrobat",
		"whitespace tab":    "acrobat\tcufflink dresser osmosis riverboat tulip wolverine",
		"crlf contaminated": "acrobat cufflink dresser osmosis riverboat tulip wolverine\n",
	} {
		if got := SeedWords([]byte(p), 0); got != nil {
			t.Errorf("%s: SeedWords(%q) = %v, want nil", name, p, got)
		}
	}
	// The gate parameter mirrors the prompt's Min.
	if got := SeedWords([]byte("cup elk"), 2); !slices.Equal(got, []string{"cup", "elk"}) {
		t.Errorf("min-2 SeedWords = %v", got)
	}
	if got := SeedWords([]byte("cup elk"), 0); got != nil {
		t.Errorf("default-gate SeedWords(cup elk) = %v, want nil", got)
	}
	if got := SeedWords([]byte(strings.Join(wordlist.Words()[:wordlist.MaxPhraseWords+1], " ")), 0); got != nil {
		t.Errorf("SeedWords accepted 65 words: %v", got)
	}
}

// --- the A9 rule, pinned over the real list --------------------------------

// A9: any printable rune is accepted iff ≥1 candidate would remain. '-' is
// accepted exactly at "yo-" (unique → ghost "yo"); digits/punctuation still
// reject everywhere else because no list word contains them.
func TestYoYoTypeable(t *testing.T) {
	m := NewMachine(0)
	m.Handle(rn('y'))
	m.Handle(rn('o'))
	out := m.Handle(rn('-'))
	if out.Bell || out.Buf != "yo-" || out.Ghost != "yo" {
		t.Fatalf("bell=%v buf=%q ghost=%q, want yo- with ghost yo", out.Bell, out.Buf, out.Ghost)
	}
	out = m.Handle(kd(KindSpace))
	if len(out.Committed) != 1 || out.Committed[0] != "yo-yo" {
		t.Fatalf("committed = %v", out.Committed)
	}
	// hyphen is NOT blanket-allowed: rejected where no hyphen word remains
	m2 := NewMachine(0)
	m2.Handle(rn('a'))
	out = m2.Handle(rn('-'))
	if !out.Bell || out.Buf != "a" {
		t.Fatalf(`"a-" must reject: bell=%v buf=%q`, out.Bell, out.Buf)
	}
	// and digits still reject even at the yo- point
	m3 := NewMachine(0)
	m3.Handle(rn('y'))
	m3.Handle(rn('o'))
	out = m3.Handle(rn('7'))
	if !out.Bell || out.Buf != "yo" {
		t.Fatalf(`"yo7" must reject: bell=%v buf=%q`, out.Bell, out.Buf)
	}
}

// Every list word is reachable by pure typing under the A9 rule: each of its
// prefixes has ≥1 candidate (itself). Verified over all 1296.
func TestReachabilityByTyping(t *testing.T) {
	var failed []string
	for _, w := range wordlist.Words() {
		m := NewMachine(0)
		ok := true
		for _, r := range w {
			if out := m.Handle(rn(r)); out.Bell {
				ok = false
				break
			}
		}
		if ok {
			if out := m.Handle(kd(KindSpace)); out.Bell || len(out.Committed) != 1 || out.Committed[0] != w {
				ok = false
			}
		}
		if !ok {
			failed = append(failed, w)
		}
	}
	if len(failed) > 0 {
		t.Fatalf("unreachable by typing: %s", strings.Join(failed, " "))
	}
}
