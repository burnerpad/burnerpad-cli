package term

import (
	"bytes"
	"reflect"
	"testing"
)

// feedAll runs the byte stream through a fresh decoder and returns every
// event (chunking is irrelevant: the decoder is fed byte-by-byte by design).
func feedAll(d *keyDecoder, b []byte) []event {
	var out []event
	for _, c := range b {
		out = append(out, d.feed(c)...)
	}
	return out
}

func TestDecoderSingleBytes(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want []event
	}{
		{"letters", []byte("acr"), []event{rn('a'), rn('c'), rn('r')}},
		{"space", []byte{' '}, []event{kd(kindSpace)}},
		{"tab", []byte{'\t'}, []event{kd(kindTab)}},
		{"CR is Enter", []byte{'\r'}, []event{kd(kindEnter)}},
		{"LF is Enter", []byte{'\n'}, []event{kd(kindEnter)}},
		{"DEL is Backspace", []byte{0x7f}, []event{kd(kindBackspace)}},
		{"BS is Backspace", []byte{0x08}, []event{kd(kindBackspace)}},
		{"Ctrl+C", []byte{0x03}, []event{kd(kindCtrlC)}},
		{"Ctrl+D", []byte{0x04}, []event{kd(kindCtrlD)}},
		{"Ctrl+O", []byte{0x0f}, []event{kd(kindCtrlO)}},
		{"Ctrl+U", []byte{0x15}, []event{kd(kindCtrlU)}},
		{"Ctrl+W", []byte{0x17}, []event{kd(kindCtrlW)}},
		{"other C0 ignored", []byte{0x01, 0x02, 0x1a}, []event{kd(kindIgnored), kd(kindIgnored), kd(kindIgnored)}},
		{"punctuation is a rune", []byte{'!'}, []event{rn('!')}},
		{"two-byte UTF-8", []byte("é"), []event{rn('é')}},
		{"three-byte UTF-8", []byte("…"), []event{rn('…')}},
		{"four-byte UTF-8", []byte("🎉"), []event{rn('🎉')}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := feedAll(newKeyDecoder(), tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("events = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestDecoderCoalescesCookedCRLF(t *testing.T) {
	got := feedAll(newKeyDecoder(), []byte("a\r\nb\n"))
	want := []event{rn('a'), kd(kindEnter), rn('b'), kd(kindEnter)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

// The §7.2 ESC row: sequences are swallowed whole — ESC [ A never injects
// an 'a', and none of the navigation keys produce anything but one ignore.
func TestDecoderSwallowsSequencesWhole(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"up arrow", []byte("\x1b[A")},
		{"down arrow", []byte("\x1b[B")},
		{"right arrow", []byte("\x1b[C")},
		{"left arrow", []byte("\x1b[D")},
		{"home CSI H", []byte("\x1b[H")},
		{"end CSI F", []byte("\x1b[F")},
		{"home tilde", []byte("\x1b[1~")},
		{"delete", []byte("\x1b[3~")},
		{"page up", []byte("\x1b[5~")},
		{"F5", []byte("\x1b[15~")},
		{"F12", []byte("\x1b[24~")},
		{"shift-arrow with params", []byte("\x1b[1;2A")},
		{"SS3 F1", []byte("\x1bOP")},
		{"SS3 keypad enter", []byte("\x1bOM")},
		{"alt-chord", []byte("\x1bx")},
		{"stray paste terminator", []byte("\x1b[201~")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := feedAll(newKeyDecoder(), tc.in)
			want := []event{kd(kindIgnored)}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("events = %+v, want exactly one ignore", got)
			}
		})
	}
}

// A sequence split across reads decodes identically: the decoder is fed
// byte-by-byte, so a chunk boundary can never tear a sequence.
func TestDecoderSplitAcrossReads(t *testing.T) {
	d := newKeyDecoder()
	var got []event
	got = append(got, feedAll(d, []byte("\x1b["))...) // partial CSI…
	if len(got) != 0 || !d.pending() {
		t.Fatalf("mid-CSI: events=%+v pending=%v", got, d.pending())
	}
	got = append(got, feedAll(d, []byte("A"))...) // …completed later
	got = append(got, feedAll(d, []byte("q"))...)
	want := []event{kd(kindIgnored), rn('q')}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}

	// split UTF-8 rune
	d = newKeyDecoder()
	got = feedAll(d, []byte{0xc3})
	if len(got) != 0 || !d.pending() {
		t.Fatalf("mid-rune: events=%+v pending=%v", got, d.pending())
	}
	got = feedAll(d, []byte{0xa9})
	if !reflect.DeepEqual(got, []event{rn('é')}) {
		t.Fatalf("events = %+v", got)
	}
}

func TestDecoderBracketedPaste(t *testing.T) {
	t.Run("simple payload", func(t *testing.T) {
		got := feedAll(newKeyDecoder(), []byte("\x1b[200~acrobat cufflink\x1b[201~q"))
		want := []event{{Kind: kindPaste, Paste: []byte("acrobat cufflink")}, rn('q')}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("events = %+v, want %+v", got, want)
		}
	})
	t.Run("payload containing ESC bytes", func(t *testing.T) {
		payload := "a\x1b[Bz\x1b"
		got := feedAll(newKeyDecoder(), []byte("\x1b[200~"+payload+"\x1b[201~"))
		want := []event{{Kind: kindPaste, Paste: []byte(payload)}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("events = %+v, want %+v", got, want)
		}
	})
	t.Run("payload with a near-terminator prefix", func(t *testing.T) {
		payload := "x\x1b[20A"
		got := feedAll(newKeyDecoder(), []byte("\x1b[200~"+payload+"\x1b[201~"))
		want := []event{{Kind: kindPaste, Paste: []byte(payload)}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("events = %+v, want %+v", got, want)
		}
	})
	t.Run("empty paste", func(t *testing.T) {
		got := feedAll(newKeyDecoder(), []byte("\x1b[200~\x1b[201~"))
		if len(got) != 1 || got[0].Kind != kindPaste || len(got[0].Paste) != 0 {
			t.Fatalf("events = %+v, want one empty kindPaste", got)
		}
	})
	t.Run("paste payload is not key-decoded", func(t *testing.T) {
		// Control bytes inside a paste stay payload, never events.
		got := feedAll(newKeyDecoder(), []byte("\x1b[200~a\x03b\r\x1b[201~"))
		want := []event{{Kind: kindPaste, Paste: []byte("a\x03b\r")}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("events = %+v, want %+v", got, want)
		}
	})
	t.Run("mid-paste is not pending (no inter-byte timeout applies)", func(t *testing.T) {
		d := newKeyDecoder()
		feedAll(d, []byte("\x1b[200~partial"))
		if d.pending() {
			t.Fatal("pending() must be false inside a bracketed paste")
		}
	})
}

func TestDecoderBoundsBracketedPasteAndRecovers(t *testing.T) {
	t.Run("exact limit", func(t *testing.T) {
		payload := bytes.Repeat([]byte{'a'}, maxPasteBytes)
		wire := append([]byte("\x1b[200~"), payload...)
		wire = append(wire, []byte(pasteEnd)...)
		got := feedAll(newKeyDecoder(), wire)
		if len(got) != 1 || got[0].Kind != kindPaste || !bytes.Equal(got[0].Paste, payload) {
			t.Fatalf("exact-limit events = %+v", got)
		}
	})

	t.Run("limit plus one", func(t *testing.T) {
		d := newKeyDecoder()
		if got := feedAll(d, []byte("\x1b[200~")); len(got) != 0 {
			t.Fatalf("paste start events = %+v", got)
		}
		if got := feedAll(d, bytes.Repeat([]byte{'s'}, maxPasteBytes)); len(got) != 0 {
			t.Fatalf("payload events = %+v", got)
		}
		if len(d.paste) != maxPasteBytes || cap(d.paste) != maxPasteBytes {
			t.Fatalf("retained paste len/cap = %d/%d, want %d/%d", len(d.paste), cap(d.paste), maxPasteBytes, maxPasteBytes)
		}
		held := d.paste
		if got := d.feed('x'); len(got) != 0 {
			t.Fatalf("overflow emitted before terminator: %+v", got)
		}
		if !d.pover || len(d.paste) != 0 {
			t.Fatalf("overflow state: pover=%v retained=%d", d.pover, len(d.paste))
		}
		if !bytes.Equal(held, make([]byte, len(held))) {
			t.Fatal("retained paste bytes were not wiped on overflow")
		}
		got := feedAll(d, append([]byte("ignored after overflow"), []byte(pasteEnd)...))
		if !reflect.DeepEqual(got, []event{kd(kindInputTooLong)}) {
			t.Fatalf("overflow completion = %+v", got)
		}
		if got := feedAll(d, []byte("q")); !reflect.DeepEqual(got, []event{rn('q')}) {
			t.Fatalf("decoder unusable after overflow: %+v", got)
		}
	})
}

func TestDecoderInvalidUTF8(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want []event
	}{
		{"invalid lead 0xFF", []byte{0xff}, []event{kd(kindIgnored)}},
		{"lone continuation", []byte{0xaf}, []event{kd(kindIgnored)}},
		{"overlong lead 0xC0", []byte{0xc0, 0xaf}, []event{kd(kindIgnored), kd(kindIgnored)}},
		{"torn rune then ASCII", []byte{0xc3, '('}, []event{kd(kindIgnored), rn('(')}},
		{"surrogate half", []byte{0xed, 0xa0, 0x80}, []event{kd(kindIgnored)}},
		{"beyond U+10FFFF lead", []byte{0xf5, 0x80}, []event{kd(kindIgnored), kd(kindIgnored)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := feedAll(newKeyDecoder(), tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("events = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The TTY side flushes on the 50 ms inter-byte timeout; flush must turn any
// mid-flight sequence into exactly one ignore and leave the decoder clean.
func TestDecoderFlush(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"lone ESC", []byte{0x1b}},
		{"partial CSI", []byte("\x1b[1;")},
		{"partial SS3", []byte("\x1bO")},
		{"partial rune", []byte{0xe2, 0x80}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newKeyDecoder()
			if evs := feedAll(d, tc.in); len(evs) != 0 {
				t.Fatalf("premature events: %+v", evs)
			}
			if !d.pending() {
				t.Fatal("decoder should be pending")
			}
			if got := d.flush(); !reflect.DeepEqual(got, []event{kd(kindIgnored)}) {
				t.Fatalf("flush = %+v", got)
			}
			if d.pending() {
				t.Fatal("still pending after flush")
			}
			if got := feedAll(d, []byte("q")); !reflect.DeepEqual(got, []event{rn('q')}) {
				t.Fatalf("decoder unusable after flush: %+v", got)
			}
		})
	}
	if got := newKeyDecoder().flush(); got != nil {
		t.Fatalf("flush at ground state = %+v, want nil", got)
	}
}

// A CSI sequence that never terminates within the 16-byte bound is abandoned
// as one ignore, and the decoder returns to ground (later bytes are new
// input).
func TestDecoderCSIBound(t *testing.T) {
	d := newKeyDecoder()
	// ESC + '[' + 15 parameter bytes = 17 > 16: the 15th parameter byte
	// trips the bound.
	in := append([]byte("\x1b["), []byte("1;1;1;1;1;1;1;1")...)
	var got []event
	for _, b := range in {
		got = append(got, d.feed(b)...)
	}
	if !reflect.DeepEqual(got, []event{kd(kindIgnored)}) {
		t.Fatalf("overlong CSI: events = %+v, want one ignore", got)
	}
	if d.pending() {
		t.Fatal("decoder stuck pending after bound hit")
	}
	if got := feedAll(d, []byte("q")); !reflect.DeepEqual(got, []event{rn('q')}) {
		t.Fatalf("decoder unusable after bound: %+v", got)
	}
}
