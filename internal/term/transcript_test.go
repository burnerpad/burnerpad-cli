package term

import (
	"reflect"
	"testing"
)

// TestTranscript10c replays the §10(c) keystroke-by-keystroke session
// exactly, asserting every annotated status/ghost/commit point:
//
//	word 1/7 ▸ a‸            status: 126 words match
//	word 1/7 ▸ ac‸           status: 10 match: academy accountant acetone …
//	word 1/7 ▸ acr‸⟨obat⟩    unique at 3 chars → ghost
//	word 1/7 ▸ acrobat ␣     Space commits
//	… cufflink, dresser, osmosis, riverboat the same way …
//	word 6/7 ▸ … tup‸⟨perware⟩  slip: valid prefix of the WRONG word; nothing committed
//	word 6/7 ▸ … tu‸         Backspace; ghost gone (tu- has 10 candidates)
//	word 6/7 ▸ … tul‸⟨ip⟩ ␣  corrected; Space commits tulip
//	word 7/7 ▸ … wa‸ + "x"   BEL, generic rejection; x never enters buf
//	word 7/7 ▸ … wol‸⟨verine⟩ ␣
//	7 words · Enter submits — keep typing if the phrase was longer
//	⏎  → decrypt
func TestTranscript10c(t *testing.T) {
	m := newMachine(0)

	// word 1: a → c → r → ghost → Space
	out := m.handle(rn('a'))
	if out.status != "126 words match" {
		t.Fatalf(`after "a": status = %q, want "126 words match"`, out.status)
	}
	out = m.handle(rn('c'))
	if out.status != "10 match: academy accountant acetone …" {
		t.Fatalf(`after "ac": status = %q`, out.status)
	}
	out = m.handle(rn('r'))
	if out.ghost != "obat" || out.buf != "acr" {
		t.Fatalf(`after "acr": buf=%q ghost=%q, want unique ghost "obat"`, out.buf, out.ghost)
	}
	out = m.handle(kd(kindSpace))
	if !reflect.DeepEqual(out.committed, []string{"acrobat"}) || out.buf != "" {
		t.Fatalf("Space after acr: committed=%v buf=%q", out.committed, out.buf)
	}

	// word 2: cu (10 match) → f → ghost flink → Space
	m.handle(rn('c'))
	out = m.handle(rn('u'))
	if out.status != "10 match: cubical cucumber cuddly …" {
		t.Fatalf(`after "cu": status = %q (transcript: "10 match")`, out.status)
	}
	out = m.handle(rn('f'))
	if out.ghost != "flink" {
		t.Fatalf(`after "cuf": ghost = %q, want "flink"`, out.ghost)
	}
	m.handle(kd(kindSpace))

	// words 3–5: dresser, osmosis, riverboat — 3 keys + Space each
	for _, w := range []struct{ keys, word string }{
		{"dre", "dresser"}, {"osm", "osmosis"}, {"riv", "riverboat"},
	} {
		for _, r := range w.keys {
			out = m.handle(rn(r))
		}
		if got := w.keys + out.ghost; got != w.word {
			t.Fatalf("typing %q: buf+ghost = %q, want %q", w.keys, got, w.word)
		}
		m.handle(kd(kindSpace))
	}
	if got := m.committedCopy(); len(got) != 5 {
		t.Fatalf("after 5 words: committed = %v", got)
	}

	// word 6 slip: meant tulip, typed "tup" — a VALID prefix of tupperware.
	// Ghost shows the wrong word in full; NOTHING committed (no auto-commit).
	m.handle(rn('t'))
	m.handle(rn('u'))
	out = m.handle(rn('p'))
	if out.ghost != "perware" {
		t.Fatalf(`slip "tup": ghost = %q, want "perware"`, out.ghost)
	}
	if len(out.committed) != 5 {
		t.Fatalf("slip must not auto-commit: committed = %v", out.committed)
	}

	// Backspace: ghost gone, tu- has 10 candidates
	out = m.handle(kd(kindBackspace))
	if out.buf != "tu" || out.ghost != "" {
		t.Fatalf("after Backspace: buf=%q ghost=%q", out.buf, out.ghost)
	}
	if n := m.candCount("tu"); n != 10 {
		t.Fatalf("tu- candidates = %d, want 10 (transcript annotation)", n)
	}

	// corrected: l → ghost ip → Space commits tulip
	out = m.handle(rn('l'))
	if out.ghost != "ip" {
		t.Fatalf(`after "tul": ghost = %q, want "ip"`, out.ghost)
	}
	m.handle(kd(kindSpace))

	// word 7: "wa" then "x" → BEL, input-free status, x never enters buf
	m.handle(rn('w'))
	m.handle(rn('a'))
	out = m.handle(rn('x'))
	if !out.bell || out.status != rejectedCharacterHint || out.buf != "wa" {
		t.Fatalf(`reject: bell=%v status=%q buf=%q`, out.bell, out.status, out.buf)
	}

	// clear the false start, type wol → ghost verine → Space commits
	m.handle(kd(kindBackspace))
	m.handle(kd(kindBackspace))
	m.handle(rn('w'))
	m.handle(rn('o'))
	out = m.handle(rn('l'))
	if out.ghost != "verine" {
		t.Fatalf(`after "wol": ghost = %q`, out.ghost)
	}
	out = m.handle(kd(kindSpace))
	if out.status != "7 words · Enter submits — keep typing if the phrase was longer" {
		t.Fatalf("7th commit status = %q", out.status)
	}
	if out.done {
		t.Fatal("Space must never trigger decrypt")
	}

	// ⏎ → attempt decrypt
	out = m.handle(kd(kindEnter))
	if !out.done {
		t.Fatal("Enter at 7 words with empty buf: done = false")
	}
	want := "acrobat cufflink dresser osmosis riverboat tulip wolverine"
	if got := string(out.phrase); got != want {
		t.Fatalf("phrase = %q, want %q", got, want)
	}
}

// Keystroke economics claim of §10(c): the 7-word happy path above is
// 3 chars + Space per word plus Enter ≈ 29 gestures. Sanity-check that the
// per-word 3-keystroke uniqueness held at every word (implied by the ghost
// assertions), and that the whole phrase needed no more than 4 gestures/word.
func TestTranscriptKeystrokeBudget(t *testing.T) {
	m := newMachine(0)
	words := []string{"acrobat", "cufflink", "dresser", "osmosis", "riverboat", "tulip", "wolverine"}
	gestures := 0
	for _, w := range words {
		for _, r := range w[:3] {
			m.handle(rn(r))
			gestures++
		}
		out := m.handle(kd(kindSpace))
		gestures++
		if out.bell {
			t.Fatalf("word %q not unique at 3 chars", w)
		}
	}
	out := m.handle(kd(kindEnter))
	gestures++
	if !out.done {
		t.Fatal("not done")
	}
	if gestures != 7*4+1 {
		t.Fatalf("gestures = %d, want 29", gestures)
	}
}
