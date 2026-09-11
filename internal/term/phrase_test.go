package term

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// pipeTTY builds a TTY over os.Pipe pairs: not a real terminal (makeRaw
// fails, Size falls back to 80×24), but the pump, decoder wiring, and the
// plain fallback are all real. Returns the TTY, the input writer, and the
// output reader.
func pipeTTY(t *testing.T) (*TTY, *os.File, *os.File) {
	t.Helper()
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	tty := newTTY(inR, outW)
	t.Cleanup(func() {
		tty.Close()
		inW.Close()
		outR.Close()
	})
	return tty, inW, outR
}

func TestReadEventPump(t *testing.T) {
	tty, inW, _ := pipeTTY(t)
	if _, err := inW.Write([]byte("ac \x1b[A\x03")); err != nil {
		t.Fatal(err)
	}
	want := []event{rn('a'), rn('c'), kd(kindSpace), kd(kindIgnored), kd(kindCtrlC)}
	for i, w := range want {
		ev, err := tty.readEventContext(context.Background())
		if err != nil {
			t.Fatalf("event %d: %v", i, err)
		}
		if ev.Kind != w.Kind || ev.R != w.R {
			t.Fatalf("event %d = %+v, want %+v", i, ev, w)
		}
	}
	inW.Close()
	if _, err := tty.readEventContext(context.Background()); err != io.EOF {
		t.Fatalf("after close: err = %v, want io.EOF", err)
	}
	// EOF is sticky.
	if _, err := tty.readEventContext(context.Background()); err != io.EOF {
		t.Fatalf("second read after EOF: err = %v", err)
	}
}

func TestReadEventContextCancellation(t *testing.T) {
	tty, _, _ := pipeTTY(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tty.readEventContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// The TTY-side 50 ms timeout: a lone ESC is delivered as ignored once the
// inter-byte deadline passes, instead of being glued to the next keystroke.
func TestReadEventEscTimeout(t *testing.T) {
	old := escTimeout
	escTimeout = 20 * time.Millisecond
	defer func() { escTimeout = old }()

	tty, inW, _ := pipeTTY(t)
	if _, err := inW.Write([]byte{0x1b}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	ev, err := tty.readEventContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != kindIgnored {
		t.Fatalf("lone ESC decoded as %+v, want ignored", ev)
	}
	if elapsed := time.Since(start); elapsed < escTimeout {
		t.Fatalf("flush before the deadline: %v", elapsed)
	}
	// The decoder is clean afterwards: the next byte is a plain rune.
	if _, err := inW.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	ev, err = tty.readEventContext(context.Background())
	if err != nil || ev.Kind != kindRune || ev.R != 'q' {
		t.Fatalf("after timeout: ev=%+v err=%v", ev, err)
	}
}

// A paste burst is NOT subject to the inter-byte timeout: split writes with
// a pause longer than escTimeout still produce one atomic kindPaste.
func TestReadEventPasteSurvivesPause(t *testing.T) {
	old := escTimeout
	escTimeout = 10 * time.Millisecond
	defer func() { escTimeout = old }()

	tty, inW, _ := pipeTTY(t)
	if _, err := inW.Write([]byte("\x1b[200~acrobat ")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := inW.Write([]byte("tulip\x1b[201~")); err != nil {
		t.Fatal(err)
	}
	ev, err := tty.readEventContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != kindPaste || string(ev.Paste) != "acrobat tulip" {
		t.Fatalf("paste = %+v", ev)
	}
}

// ReadPhrase over a non-terminal falls back to plain line mode, as it does
// when the raw-mode probe fails on legacy conhost, and still produces the
// canonical phrase.
func TestReadPhraseFallsBackToPlain(t *testing.T) {
	tty, inW, outR := pipeTTY(t)
	go func() {
		io.WriteString(inW, "acrobat cufflink dresser osmosis riverboat tulip wolverine\n\n")
		inW.Close()
	}()
	buf, err := ReadPhraseContext(context.Background(), tty, PhraseOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer buf.Wipe()
	if got := string(buf.Bytes()); got != "acrobat cufflink dresser osmosis riverboat tulip wolverine" {
		t.Fatalf("phrase = %q", got)
	}
	tty.out.Close()
	transcript, _ := io.ReadAll(outR)
	if !strings.Contains(string(transcript), "Passphrase — one word per line") {
		t.Fatalf("plain intro missing: %q", transcript)
	}
}

// Plain is also honored when asked for explicitly.
func TestReadPhraseExplicitPlain(t *testing.T) {
	tty, inW, _ := pipeTTY(t)
	go func() {
		io.WriteString(inW, mintPhrase+"\n\n")
		inW.Close()
	}()
	buf, err := ReadPhraseContext(context.Background(), tty, PhraseOpts{Plain: true})
	if err != nil {
		t.Fatal(err)
	}
	defer buf.Wipe()
	if got := string(buf.Bytes()); got != mintPhrase {
		t.Fatalf("phrase = %q", got)
	}
}

func TestReadPhrasePlainInterrupt(t *testing.T) {
	tty, inW, _ := pipeTTY(t)
	inW.Close() // immediate EOF at the prompt
	if _, err := ReadPhraseContext(context.Background(), tty, PhraseOpts{Plain: true}); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("err = %v, want ErrInterrupted", err)
	}
}

func TestReadPhrasePlainContextCancellation(t *testing.T) {
	tty, _, _ := pipeTTY(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadPhraseContext(ctx, tty, PhraseOpts{Plain: true}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestEmergencyRestoreWritesOnlyActiveModes(t *testing.T) {
	tty, _, outR := pipeTTY(t)
	tty.EmergencyRestore()
	tty.mu.Lock()
	tty.bracketedPaste = true
	tty.alternateScreen = true
	tty.mu.Unlock()
	tty.EmergencyRestore()
	tty.EmergencyRestore()
	if err := tty.out.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(outR)
	if err != nil {
		t.Fatal(err)
	}
	if want := "\x1b[?2004l\x1b[?1049l"; string(got) != want {
		t.Fatalf("cleanup = %q, want %q", got, want)
	}
}

// size falls back to 80×24 when the fd is not a terminal.
func TestSizeFallback(t *testing.T) {
	tty, _, _ := pipeTTY(t)
	w, h := tty.size()
	if w != 80 || h != 24 {
		t.Fatalf("size = %d×%d, want 80×24", w, h)
	}
}
