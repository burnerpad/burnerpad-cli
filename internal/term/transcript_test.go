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
//	word 7/7 ▸ … wa‸ + "x"   BEL, status: no word starts with "wax"; x never enters buf
//	word 7/7 ▸ … wol‸⟨verine⟩ ␣
//	7 words · Enter submits — keep typing if the phrase was longer
//	⏎  → decrypt
func TestTranscript10c(t *testing.T) {
	m := NewMachine(0)

	// word 1: a → c → r → ghost → Space
	out := m.Handle(rn('a'))
	if out.Status != "126 words match" {
		t.Fatalf(`after "a": status = %q, want "126 words match"`, out.Status)
	}
	out = m.Handle(rn('c'))
	if out.Status != "10 match: academy accountant acetone …" {
		t.Fatalf(`after "ac": status = %q`, out.Status)
	}
	out = m.Handle(rn('r'))
	if out.Ghost != "obat" || out.Buf != "acr" {
		t.Fatalf(`after "acr": buf=%q ghost=%q, want unique ghost "obat"`, out.Buf, out.Ghost)
	}
	out = m.Handle(kd(KindSpace))
	if !reflect.DeepEqual(out.Committed, []string{"acrobat"}) || out.Buf != "" {
		t.Fatalf("Space after acr: committed=%v buf=%q", out.Committed, out.Buf)
	}

	// word 2: cu (10 match) → f → ghost flink → Space
	m.Handle(rn('c'))
	out = m.Handle(rn('u'))
	if out.Status != "10 match: cubical cucumber cuddly …" {
		t.Fatalf(`after "cu": status = %q (transcript: "10 match")`, out.Status)
	}
	out = m.Handle(rn('f'))
	if out.Ghost != "flink" {
		t.Fatalf(`after "cuf": ghost = %q, want "flink"`, out.Ghost)
	}
	m.Handle(kd(KindSpace))

	// words 3–5: dresser, osmosis, riverboat — 3 keys + Space each
	for _, w := range []struct{ keys, word string }{
		{"dre", "dresser"}, {"osm", "osmosis"}, {"riv", "riverboat"},
	} {
		for _, r := range w.keys {
			out = m.Handle(rn(r))
		}
		if got := w.keys + out.Ghost; got != w.word {
			t.Fatalf("typing %q: buf+ghost = %q, want %q", w.keys, got, w.word)
		}
		m.Handle(kd(KindSpace))
	}
	if got := m.Committed(); len(got) != 5 {
		t.Fatalf("after 5 words: committed = %v", got)
	}

	// word 6 slip: meant tulip, typed "tup" — a VALID prefix of tupperware.
	// Ghost shows the wrong word in full; NOTHING committed (no auto-commit).
	m.Handle(rn('t'))
	m.Handle(rn('u'))
	out = m.Handle(rn('p'))
	if out.Ghost != "perware" {
		t.Fatalf(`slip "tup": ghost = %q, want "perware"`, out.Ghost)
	}
	if len(out.Committed) != 5 {
		t.Fatalf("slip must not auto-commit: committed = %v", out.Committed)
	}

	// Backspace: ghost gone, tu- has 10 candidates
	out = m.Handle(kd(KindBackspace))
	if out.Buf != "tu" || out.Ghost != "" {
		t.Fatalf("after Backspace: buf=%q ghost=%q", out.Buf, out.Ghost)
	}
	if n := m.candCount("tu"); n != 10 {
		t.Fatalf("tu- candidates = %d, want 10 (transcript annotation)", n)
	}

	// corrected: l → ghost ip → Space commits tulip
	out = m.Handle(rn('l'))
	if out.Ghost != "ip" {
		t.Fatalf(`after "tul": ghost = %q, want "ip"`, out.Ghost)
	}
	m.Handle(kd(KindSpace))

	// word 7: "wa" then "x" → BEL, status names "wax", x never enters buf
	m.Handle(rn('w'))
	m.Handle(rn('a'))
	out = m.Handle(rn('x'))
	if !out.Bell || out.Status != `no word starts with "wax"` || out.Buf != "wa" {
		t.Fatalf(`reject: bell=%v status=%q buf=%q`, out.Bell, out.Status, out.Buf)
	}

	// clear the false start, type wol → ghost verine → Space commits
	m.Handle(kd(KindBackspace))
	m.Handle(kd(KindBackspace))
	m.Handle(rn('w'))
	m.Handle(rn('o'))
	out = m.Handle(rn('l'))
	if out.Ghost != "verine" {
		t.Fatalf(`after "wol": ghost = %q`, out.Ghost)
	}
	out = m.Handle(kd(KindSpace))
	if out.Status != "7 words · Enter submits — keep typing if the phrase was longer" {
		t.Fatalf("7th commit status = %q", out.Status)
	}
	if out.Done {
		t.Fatal("Space must never trigger decrypt")
	}

	// ⏎ → attempt decrypt
	out = m.Handle(kd(KindEnter))
	if !out.Done {
		t.Fatal("Enter at 7 words with empty buf: done = false")
	}
	want := "acrobat cufflink dresser osmosis riverboat tulip wolverine"
	if got := string(out.Phrase); got != want {
		t.Fatalf("phrase = %q, want %q", got, want)
	}
}

// Keystroke economics claim of §10(c): the 7-word happy path above is
// 3 chars + Space per word plus Enter ≈ 29 gestures. Sanity-check that the
// per-word 3-keystroke uniqueness held at every word (implied by the ghost
// assertions), and that the whole phrase needed no more than 4 gestures/word.
func TestTranscriptKeystrokeBudget(t *testing.T) {
	m := NewMachine(0)
	words := []string{"acrobat", "cufflink", "dresser", "osmosis", "riverboat", "tulip", "wolverine"}
	gestures := 0
	for _, w := range words {
		for _, r := range w[:3] {
			m.Handle(rn(r))
			gestures++
		}
		out := m.Handle(kd(KindSpace))
		gestures++
		if out.Bell {
			t.Fatalf("word %q not unique at 3 chars", w)
		}
	}
	out := m.Handle(kd(KindEnter))
	gestures++
	if !out.Done {
		t.Fatal("not done")
	}
	if gestures != 7*4+1 {
		t.Fatalf("gestures = %d, want 29", gestures)
	}
}
