package term

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type failingOSC52Writer struct{}

func (failingOSC52Writer) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestOSC52ReportsWriteFailure(t *testing.T) {
	if err := OSC52Copy(failingOSC52Writer{}, []byte("secret")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("error=%v, want closed pipe", err)
	}
}

func TestOSC52CopyGolden(t *testing.T) {
	t.Setenv("TMUX", "")
	var buf bytes.Buffer
	if err := OSC52Copy(&buf, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	want := "\x1b]52;c;aGk=\a"
	if got := buf.String(); got != want {
		t.Fatalf("sequence = %q, want %q", got, want)
	}
}

func TestOSC52ClearGolden(t *testing.T) {
	t.Setenv("TMUX", "")
	var buf bytes.Buffer
	if err := OSC52Clear(&buf); err != nil {
		t.Fatal(err)
	}
	want := "\x1b]52;c;\a" // empty payload IS the clear (§8.1)
	if got := buf.String(); got != want {
		t.Fatalf("sequence = %q, want %q", got, want)
	}
}

func TestOSC52TmuxPassthrough(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
	var buf bytes.Buffer
	if err := OSC52Copy(&buf, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	// ESC Ptmux; <inner with ESC doubled> ESC \
	want := "\x1bPtmux;\x1b\x1b]52;c;aGk=\a\x1b\\"
	if got := buf.String(); got != want {
		t.Fatalf("sequence = %q, want %q", got, want)
	}
}

func TestOSC52TmuxClear(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
	var buf bytes.Buffer
	if err := OSC52Clear(&buf); err != nil {
		t.Fatal(err)
	}
	want := "\x1bPtmux;\x1b\x1b]52;c;\a\x1b\\"
	if got := buf.String(); got != want {
		t.Fatalf("sequence = %q, want %q", got, want)
	}
}

// Binary payloads survive: base64 is what makes OSC 52 8-bit-safe.
func TestOSC52BinaryPayload(t *testing.T) {
	t.Setenv("TMUX", "")
	var buf bytes.Buffer
	if err := OSC52Copy(&buf, []byte{0x00, 0x1b, 0xff}); err != nil {
		t.Fatal(err)
	}
	want := "\x1b]52;c;ABv/\a"
	if got := buf.String(); got != want {
		t.Fatalf("sequence = %q, want %q", got, want)
	}
}
