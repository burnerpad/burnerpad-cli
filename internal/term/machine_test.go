package term

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/burnerpad/burnerpad-cli/wordlist"
)

// --- tiny step DSL for table-driven event tests (ported from the prototype) --

func rn(r rune) event      { return event{Kind: kindRune, R: r} }
func kd(k eventKind) event { return event{Kind: k} }
func paste(s string) event { return event{Kind: kindPaste, Paste: []byte(s)} }

type expect struct {
	buf       *string
	ghost     *string
	bell      *bool
	status    *string
	committed []string // nil = don't check
	done      *bool
}

type step struct {
	ev event
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

func runSteps(t *testing.T, m *machine, steps []step) {
	t.Helper()
	for i, st := range steps {
		out := m.handle(st.ev)
		if st.buf != nil && out.buf != *st.buf {
			t.Fatalf("step %d (%+v): buf = %q, want %q", i, st.ev, out.buf, *st.buf)
		}
		if st.ghost != nil && out.ghost != *st.ghost {
			t.Fatalf("step %d (%+v): ghost = %q, want %q", i, st.ev, out.ghost, *st.ghost)
		}
		if st.bell != nil && out.bell != *st.bell {
			t.Fatalf("step %d (%+v): bell = %v, want %v (status %q)", i, st.ev, out.bell, *st.bell, out.status)
		}
		if st.status != nil && out.status != *st.status {
			t.Fatalf("step %d (%+v): status = %q, want %q", i, st.ev, out.status, *st.status)
		}
		if st.committed != nil && !slices.Equal(out.committed, st.committed) {
			t.Fatalf("step %d (%+v): committed = %v, want %v", i, st.ev, out.committed, st.committed)
		}
		if st.done != nil && out.done != *st.done {
			t.Fatalf("step %d (%+v): done = %v, want %v", i, st.ev, out.done, *st.done)
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
					buf: sp("ac"), bell: bp(true), status: sp(rejectedCharacterHint)}},
			},
		},
		{
			name: "uppercase lowercased silently",
			steps: append(typeRunes("ACR"),
				step{ev: kd(kindSpace), expect: expect{committed: []string{"acrobat"}, buf: sp("")}}),
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
					status: sp(rejectedCharacterHint)}}),
		},
		{
			name: "punctuation (non-hyphen) rejected in list-locked mode",
			steps: append(typeRunes("wa"),
				step{ev: rn('!'), expect: expect{buf: sp("wa"), bell: bp(true),
					status: sp(rejectedCharacterHint)}}),
		},
		{
			name: "candidates==1 shows remainder as ghost",
			steps: append(typeRunes("tu"),
				step{ev: rn('l'), expect: expect{buf: sp("tul"), ghost: sp("ip")}}),
		},
		{
			name: "Space commits unique word; counter advances via committed",
			steps: append(typeRunes("acr"),
				step{ev: kd(kindSpace), expect: expect{committed: []string{"acrobat"}, buf: sp(""), ghost: sp(""), bell: bp(false)}}),
		},
		{
			name: "Space on ambiguous buf bells with first<=5 candidates",
			steps: append(typeRunes("ap"),
				// prefix "ap": apartment apnea apostrophe apple apricot — exactly 5, no ellipsis
				step{ev: kd(kindSpace), expect: expect{bell: bp(true), buf: sp("ap"),
					status: sp("still ambiguous: apartment apnea apostrophe apple apricot")}}),
		},
		{
			name: "Space on ambiguous buf with >5 candidates gets ellipsis",
			steps: append(typeRunes("a"),
				step{ev: kd(kindSpace), expect: expect{bell: bp(true),
					status: sp("still ambiguous: aardvark abandoned abbreviate abdomen abhorrence …")}}),
		},
		{
			name: "Space on empty buf ignored",
			steps: []step{
				{ev: kd(kindSpace), expect: expect{buf: sp(""), bell: bp(false), status: sp(""), committed: []string{}}},
			},
		},
		{
			name: "Tab extends to longest common prefix (q -> qu, still 4 candidates)",
			steps: []step{
				{ev: rn('q'), expect: expect{buf: sp("q")}},
				{ev: kd(kindTab), expect: expect{buf: sp("qu"), committed: []string{},
					status: sp("4 match: quarters quesadilla quilt …")}},
			},
		},
		{
			name: "Tab accepts a unique candidate: commits it and clears buf for the next word",
			steps: append(typeRunes("aq"), // aquamarine is unique at "aq"
				step{ev: kd(kindTab), expect: expect{committed: []string{"aquamarine"}, buf: sp(""),
					ghost: sp(""), bell: bp(false), status: sp("")}},
				// buf is empty, so the next rune starts word 2 with no Space needed
				step{ev: rn('t'), expect: expect{committed: []string{"aquamarine"}, buf: sp("t")}}),
		},
		{
			name: "Tab-committed word excludes itself from later candidates (like Space)",
			steps: append(typeRunes("aq"),
				step{ev: kd(kindTab), expect: expect{committed: []string{"aquamarine"}}},
				step{ev: rn('a'), expect: expect{buf: sp("a")}},
				step{ev: rn('q'), expect: expect{buf: sp("a"), bell: bp(true),
					status: sp(rejectedCharacterHint)}}),
		},
		{
			name: "Tab at the gate reports the same commit status Space does",
			steps: append([]step{{ev: paste("acrobat blender cufflink dishcloth eggnog fondue")}},
				append(typeRunes("tul"),
					step{ev: kd(kindTab), expect: expect{buf: sp(""), committed: []string{"acrobat",
						"blender", "cufflink", "dishcloth", "eggnog", "fondue", "tulip"},
						status: sp("7 words · " + submitHint)}})...),
		},
		{
			name: "Tab on empty buf is a no-op",
			steps: []step{
				{ev: kd(kindTab), expect: expect{buf: sp(""), bell: bp(false), committed: []string{}}},
			},
		},
		{
			name: "Enter commits unique buf ONLY; below 7 words reports the count (A12+B3)",
			steps: append(typeRunes("acr"),
				step{ev: kd(kindEnter), expect: expect{committed: []string{"acrobat"}, buf: sp(""),
					done: bp(false), status: sp("1/7 — need at least 7 words" + ctrlOHint)}}),
		},
		{
			name: "Enter with empty buf below 7 words: status, no bell, no done",
			steps: []step{
				{ev: kd(kindEnter), expect: expect{done: bp(false), bell: bp(false),
					status: sp("0/7 — need at least 7 words" + ctrlOHint)}},
			},
		},
		{
			name: "Enter on ambiguous buf bells",
			steps: append(typeRunes("ap"),
				step{ev: kd(kindEnter), expect: expect{bell: bp(true), done: bp(false), buf: sp("ap")}}),
		},
		{
			name: "Backspace deletes last char of buf",
			steps: append(typeRunes("tup"),
				step{ev: kd(kindBackspace), expect: expect{buf: sp("tu"), ghost: sp("")}}),
		},
		{
			name: "Backspace on empty buf un-commits previous word into buf",
			steps: append(typeRunes("acr"),
				step{ev: kd(kindSpace), expect: expect{committed: []string{"acrobat"}}},
				step{ev: kd(kindBackspace), expect: expect{committed: []string{}, buf: sp("acrobat")}},
				// the un-committed word is editable text: shave it down and go elsewhere
				step{ev: kd(kindBackspace), expect: expect{buf: sp("acroba")}},
			),
		},
		{
			name: "Backspace on empty buf with nothing committed is a no-op",
			steps: []step{
				{ev: kd(kindBackspace), expect: expect{buf: sp(""), bell: bp(false), committed: []string{}}},
			},
		},
		{
			name: "Ctrl+W clears buf; committed untouched",
			steps: append(typeRunes("acr"),
				step{ev: kd(kindCtrlW), expect: expect{buf: sp(""), committed: []string{}}}),
		},
		{
			name: "Ctrl+W on empty buf deletes last committed word entirely",
			steps: []step{
				{ev: paste("acrobat cufflink"), expect: expect{committed: []string{"acrobat", "cufflink"}}},
				{ev: kd(kindCtrlW), expect: expect{committed: []string{"acrobat"}, buf: sp("")}},
			},
		},
		{
			name: "Ctrl+U clears buf only",
			steps: []step{
				{ev: paste("acrobat")},
				{ev: rn('t'), expect: expect{buf: sp("t")}},
				{ev: kd(kindCtrlU), expect: expect{buf: sp(""), committed: []string{"acrobat"}}},
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
				{ev: kd(kindCtrlO), expect: expect{buf: sp(""), bell: bp(true),
					status: sp("every passphrase word must be on the Burnerpad word list")}},
			},
		},
		{
			name: "rejections never advertise a free-form escape",
			steps: append(typeRunes("wa"),
				step{ev: rn('x'), expect: expect{bell: bp(true), status: sp(rejectedCharacterHint)}},
				step{ev: rn('x'), expect: expect{bell: bp(true),
					status: sp(rejectedCharacterHint)}}),
		},
		{
			name: "committed words are excluded from candidates",
			steps: []step{
				{ev: paste("apple")},
				// "ap" now has 4 candidates, not 5
				{ev: rn('a')}, {ev: rn('p'), expect: expect{
					status: sp("4 match: apartment apnea apostrophe …")}},
				// "app" only matched apple, which is committed, so it is unavailable.
				{ev: rn('p'), expect: expect{buf: sp("ap"), bell: bp(true),
					status: sp(rejectedCharacterHint)}},
			},
		},
		{
			name: "3-char word: unique at keystroke 3 with EMPTY ghost, Space commits",
			steps: append(typeRunes("cup"),
				step{ev: kd(kindSpace), expect: expect{committed: []string{"cup"}, buf: sp("")}}),
		},
		{
			name: "Ctrl+C, Ctrl+D and ignored events are state no-ops",
			steps: append(typeRunes("acr"),
				step{ev: kd(kindCtrlC), expect: expect{buf: sp("acr"), ghost: sp("obat"), bell: bp(false)}},
				step{ev: kd(kindCtrlD), expect: expect{buf: sp("acr")}},
				step{ev: kd(kindIgnored), expect: expect{buf: sp("acr"), committed: []string{}}}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runSteps(t, newMachine(0), tc.steps)
		})
	}
}

// A12: Enter with a non-empty buf commits ONLY — even the 7th word. The
// submit is always a second, deliberate Enter on an empty buf. (The
// prototype allowed commit+submit in one keystroke; amendment A12 overrode
// that, and this test pins the amended rule.)
func TestEnterCommitsSeventhWordThenSecondEnterSubmits(t *testing.T) {
	m := newMachine(0)
	out := m.handle(paste("acrobat cufflink dresser osmosis riverboat tulip"))
	if len(out.committed) != 6 {
		t.Fatalf("setup: committed %v", out.committed)
	}
	runSteps(t, m, typeRunes("wol"))
	out = m.handle(kd(kindEnter))
	if out.done {
		t.Fatal("A12: Enter with non-empty buf must commit only, never submit")
	}
	if want := "7 words · Enter submits — keep typing if the phrase was longer"; out.status != want {
		t.Fatalf("commit status = %q, want %q", out.status, want)
	}
	out = m.handle(kd(kindEnter))
	if !out.done {
		t.Fatal("second Enter with empty buf at 7 words: done = false")
	}
	want := "acrobat cufflink dresser osmosis riverboat tulip wolverine"
	if got := string(out.phrase); got != want {
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
	m := newMachine(0)
	m.handle(paste("acrobat cufflink dresser osmosis riverboat tulip"))
	out := m.handle(kd(kindEnter))
	if out.done || out.status != "6/7 — need at least 7 words"+ctrlOHint {
		t.Fatalf("at 6 words: done=%v status=%q", out.done, out.status)
	}
	m.handle(paste("wolverine"))
	out = m.handle(kd(kindEnter))
	if !out.done {
		t.Fatalf("at 7 words + empty buf: done = false")
	}
}

// An 8th word can be committed and Enter still submits (phrases longer than
// 7 are legal; only ≥7 is guaranteed).
func TestEighthWordThenEnter(t *testing.T) {
	m := newMachine(0)
	m.handle(paste("acrobat cufflink dresser osmosis riverboat tulip wolverine"))
	runSteps(t, m, typeRunes("zeb"))
	out := m.handle(kd(kindSpace))
	if out.status != "8 words · Enter submits — keep typing if the phrase was longer" {
		t.Fatalf("8th commit status = %q", out.status)
	}
	if out.done {
		t.Fatal("Space must never trigger decrypt")
	}
	out = m.handle(kd(kindEnter))
	if !out.done || string(out.phrase) != "acrobat cufflink dresser osmosis riverboat tulip wolverine zebra" {
		t.Fatalf("done=%v phrase=%q", out.done, out.phrase)
	}
}

// newMachine's gate is parameterized for focused terminal tests.
func TestCustomMinGate(t *testing.T) {
	m := newMachine(3)
	m.handle(paste("acrobat cufflink"))
	out := m.handle(kd(kindEnter))
	if out.done || out.status != "2/3 — need at least 3 words"+ctrlOHint {
		t.Fatalf("at 2/3: done=%v status=%q", out.done, out.status)
	}
	out = m.handle(paste("dresser"))
	if want := "3 words · Enter submits — keep typing if the phrase was longer"; out.status != want {
		t.Fatalf("3rd commit status = %q", out.status)
	}
	out = m.handle(kd(kindEnter))
	if !out.done || string(out.phrase) != "acrobat cufflink dresser" {
		t.Fatalf("done=%v phrase=%q", out.done, out.phrase)
	}
}

// machineOutput.committed must be a defensive copy.
func TestOutputCommittedIsACopy(t *testing.T) {
	m := newMachine(0)
	out := m.handle(paste("acrobat cufflink"))
	out.committed[0] = "corrupted"
	if got := m.committedCopy()[0]; got != "acrobat" {
		t.Fatalf("machine state aliased by machineOutput.committed: %q", got)
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
		m := newMachine(0)
		for _, r := range p {
			m.handle(rn(r))
		}
		before := m.candCount(string(m.buf))
		want := m.firstCandidates(string(m.buf), 1)
		out := m.handle(kd(kindTab))
		if before == 1 {
			unique++
			if out.buf != "" || !slices.Equal(out.committed, want) {
				t.Fatalf("prefix %q: Tab on a unique candidate gave committed=%v buf=%q, want %v and an empty buf",
					p, out.committed, out.buf, want)
			}
			continue
		}
		ambiguous++
		if len(out.committed) != 0 {
			t.Fatalf("prefix %q: Tab committed %v with %d candidates", p, out.committed, before)
		}
		if after := m.candCount(out.buf); before != after {
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
	m := newMachine(0)
	m.handle(paste("acrobat tupperware"))
	m.handle(kd(kindBackspace)) // "tupperware" back into buf
	for range len("tupperware") - len("tu") {
		m.handle(kd(kindBackspace))
	}
	out := m.handle(rn('l'))
	if out.buf != "tul" || out.ghost != "ip" {
		t.Fatalf("buf=%q ghost=%q", out.buf, out.ghost)
	}
	out = m.handle(kd(kindSpace))
	want := []string{"acrobat", "tulip"}
	if !reflect.DeepEqual(out.committed, want) {
		t.Fatalf("committed = %v, want %v", out.committed, want)
	}
}

// Sanity: after every event of a long random-ish walk, a non-empty buf in
// list-locked mode always has ≥1 candidate (the machine's core invariant).
func TestBufAlwaysHasCandidates(t *testing.T) {
	m := newMachine(0)
	events := []event{
		rn('a'), rn('c'), rn('r'), kd(kindSpace), rn('z'), kd(kindTab), rn('9'),
		kd(kindBackspace), kd(kindBackspace), rn('q'), kd(kindTab), kd(kindCtrlW), kd(kindBackspace),
		rn('x'), kd(kindEnter), kd(kindCtrlU), paste("tulip"), kd(kindBackspace), kd(kindSpace),
	}
	for i, e := range events {
		m.handle(e)
		if string(m.buf) != "" && m.candCount(string(m.buf)) < 1 {
			t.Fatalf("after event %d (%+v): buf %q has no candidates", i, e, m.buf)
		}
	}
}

// --- the A9 rule, pinned over the real list --------------------------------

// A9: any printable rune is accepted iff ≥1 candidate would remain. '-' is
// accepted exactly at "yo-" (unique → ghost "yo"); digits/punctuation still
// reject everywhere else because no list word contains them.
func TestYoYoTypeable(t *testing.T) {
	m := newMachine(0)
	m.handle(rn('y'))
	m.handle(rn('o'))
	out := m.handle(rn('-'))
	if out.bell || out.buf != "yo-" || out.ghost != "yo" {
		t.Fatalf("bell=%v buf=%q ghost=%q, want yo- with ghost yo", out.bell, out.buf, out.ghost)
	}
	out = m.handle(kd(kindSpace))
	if len(out.committed) != 1 || out.committed[0] != "yo-yo" {
		t.Fatalf("committed = %v", out.committed)
	}
	// hyphen is NOT blanket-allowed: rejected where no hyphen word remains
	m2 := newMachine(0)
	m2.handle(rn('a'))
	out = m2.handle(rn('-'))
	if !out.bell || out.buf != "a" {
		t.Fatalf(`"a-" must reject: bell=%v buf=%q`, out.bell, out.buf)
	}
	// and digits still reject even at the yo- point
	m3 := newMachine(0)
	m3.handle(rn('y'))
	m3.handle(rn('o'))
	out = m3.handle(rn('7'))
	if !out.bell || out.buf != "yo" {
		t.Fatalf(`"yo7" must reject: bell=%v buf=%q`, out.bell, out.buf)
	}
}

// Every list word is reachable by pure typing under the A9 rule: each of its
// prefixes has ≥1 candidate (itself). Verified over all 1296.
func TestReachabilityByTyping(t *testing.T) {
	var failed []string
	for _, w := range wordlist.Words() {
		m := newMachine(0)
		ok := true
		for _, r := range w {
			if out := m.handle(rn(r)); out.bell {
				ok = false
				break
			}
		}
		if ok {
			if out := m.handle(kd(kindSpace)); out.bell || len(out.committed) != 1 || out.committed[0] != w {
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
