package term

import (
	"bytes"
	"io"
	"testing"
)

// Secret-hygiene regressions (§12): every path that drops a paste payload
// without a consumer must wipe it itself.

// An unterminated bracketed paste abandoned by flush (EOF mid-payload) never
// reaches a consumer — flush must zero the collected bytes.
func TestFlushWipesAbandonedPaste(t *testing.T) {
	d := newKeyDecoder()
	for _, b := range []byte("\x1b[200~correct horse") {
		if evs := d.feed(b); len(evs) != 0 {
			t.Fatalf("unexpected events mid-paste: %v", evs)
		}
	}
	if d.st != dPaste || len(d.paste) == 0 {
		t.Fatalf("decoder not mid-paste: st=%v len=%d", d.st, len(d.paste))
	}
	held := d.paste // alias the payload buffer before it is abandoned
	evs := d.flush()
	if len(evs) != 1 || evs[0].Kind != KindIgnored {
		t.Fatalf("flush events = %v, want one KindIgnored", evs)
	}
	for i, b := range held {
		if b != 0 {
			t.Fatalf("abandoned paste byte %d not wiped: %q", i, held)
		}
	}
}

// A completed paste must reach the consumer INTACT (the wipe in flush must
// never touch a delivered payload).
func TestCompletedPasteSurvivesDelivery(t *testing.T) {
	d := newKeyDecoder()
	var got []Event
	for _, b := range []byte("\x1b[200~tulip\x1b[201~") {
		got = append(got, d.feed(b)...)
	}
	if len(got) != 1 || got[0].Kind != KindPaste || string(got[0].Paste) != "tulip" {
		t.Fatalf("paste delivery = %v", got)
	}
}

// The viewer ignores paste events — but it is their consumer, so it must
// wipe them (§12: "the consumer wipes it").
func TestViewerWipesPasteEvents(t *testing.T) {
	payload := []byte("pasted secret material")
	events := []Event{
		{Kind: KindPaste, Paste: payload},
		{Kind: KindRune, R: 'q'},
	}
	i := 0
	next := func() (Event, error) {
		ev := events[i]
		i++
		return ev, nil
	}
	var out bytes.Buffer
	if err := showViewer(&out, next, []byte("body"), ViewerOpts{}); err != nil {
		t.Fatalf("showViewer: %v", err)
	}
	for j, b := range payload {
		if b != 0 {
			t.Fatalf("viewer paste byte %d not wiped: %q", j, payload)
		}
	}
}

func TestPlainPhraseWipesAcceptedAndRejectedLineBuffers(t *testing.T) {
	rejected := []byte("distinctive-private-canary")
	accepted := []byte("acrobat cufflink dresser osmosis riverboat tulip wolverine")
	lines := [][]byte{rejected, accepted, {}}
	next := 0
	phrase, err := readPhrasePlainLines(io.Discard, 7, nil, func() ([]byte, error) {
		line := lines[next]
		next++
		return line, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	phrase.Wipe()
	for name, line := range map[string][]byte{"rejected": rejected, "accepted": accepted} {
		for i, b := range line {
			if b != 0 {
				t.Fatalf("%s line byte %d was not wiped", name, i)
			}
		}
	}
}
