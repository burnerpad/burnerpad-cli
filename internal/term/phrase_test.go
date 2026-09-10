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

// pipeTTY builds a TTY over os.Pipe pairs: not a real terminal (MakeRaw
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
	want := []Event{rn('a'), rn('c'), kd(KindSpace), kd(KindIgnored), kd(KindCtrlC)}
	for i, w := range want {
		ev, err := tty.ReadEvent()
		if err != nil {
			t.Fatalf("event %d: %v", i, err)
		}
		if ev.Kind != w.Kind || ev.R != w.R {
			t.Fatalf("event %d = %+v, want %+v", i, ev, w)
		}
	}
	inW.Close()
	if _, err := tty.ReadEvent(); err != io.EOF {
		t.Fatalf("after close: err = %v, want io.EOF", err)
	}
	// EOF is sticky.
	if _, err := tty.ReadEvent(); err != io.EOF {
		t.Fatalf("second read after EOF: err = %v", err)
	}
}

func TestReadEventContextCancellation(t *testing.T) {
	tty, _, _ := pipeTTY(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tty.ReadEventContext(ctx); !errors.Is(err, context.Canceled) {
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
	ev, err := tty.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindIgnored {
		t.Fatalf("lone ESC decoded as %+v, want ignored", ev)
	}
	if elapsed := time.Since(start); elapsed < escTimeout {
		t.Fatalf("flush before the deadline: %v", elapsed)
	}
	// The decoder is clean afterwards: the next byte is a plain rune.
	if _, err := inW.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	ev, err = tty.ReadEvent()
	if err != nil || ev.Kind != KindRune || ev.R != 'q' {
		t.Fatalf("after timeout: ev=%+v err=%v", ev, err)
	}
}

// A paste burst is NOT subject to the inter-byte timeout: split writes with
// a pause longer than escTimeout still produce one atomic KindPaste.
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
	ev, err := tty.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindPaste || string(ev.Paste) != "acrobat tulip" {
		t.Fatalf("paste = %+v", ev)
	}
}

// ReadPhrase over a non-terminal falls back to plain line mode (§7.6: the
// raw-mode probe failing IS the conhost/--plain trigger) and still produces
// the canonical phrase.
func TestReadPhraseFallsBackToPlain(t *testing.T) {
	tty, inW, outR := pipeTTY(t)
	go func() {
		io.WriteString(inW, "acrobat cufflink dresser osmosis riverboat tulip wolverine\n\n")
		inW.Close()
	}()
	buf, err := ReadPhrase(tty, PhraseOpts{})
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

// Plain is also honored when asked for explicitly, with Min flowing through.
func TestReadPhraseExplicitPlain(t *testing.T) {
	tty, inW, _ := pipeTTY(t)
	go func() {
		io.WriteString(inW, "cup elk\n\n")
		inW.Close()
	}()
	buf, err := ReadPhrase(tty, PhraseOpts{Plain: true, Min: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer buf.Wipe()
	if got := string(buf.Bytes()); got != "cup elk" {
		t.Fatalf("phrase = %q", got)
	}
}

// PhraseOpts.Seed (§7.3 retry) flows through ReadPhrase into the plain path:
// the kept words are echoed and an empty line resubmits them.
func TestReadPhraseSeedReachesPlain(t *testing.T) {
	seed := strings.Split("acrobat cufflink dresser osmosis riverboat tulip wolverine", " ")
	tty, inW, outR := pipeTTY(t)
	go func() {
		io.WriteString(inW, "\n")
		inW.Close()
	}()
	buf, err := ReadPhrase(tty, PhraseOpts{Plain: true, Seed: seed})
	if err != nil {
		t.Fatal(err)
	}
	defer buf.Wipe()
	if got := string(buf.Bytes()); got != "acrobat cufflink dresser osmosis riverboat tulip wolverine" {
		t.Fatalf("phrase = %q", got)
	}
	tty.out.Close()
	transcript, _ := io.ReadAll(outR)
	if !strings.Contains(string(transcript), "7 words kept: acrobat cufflink dresser osmosis riverboat tulip wolverine") {
		t.Fatalf("kept-words echo missing: %q", transcript)
	}
}

func TestReadPhrasePlainInterrupt(t *testing.T) {
	tty, inW, _ := pipeTTY(t)
	inW.Close() // immediate EOF at the prompt
	if _, err := ReadPhrase(tty, PhraseOpts{Plain: true}); !errors.Is(err, ErrInterrupted) {
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

// Out() must hand back the tty writer (prompts never touch stdout, §5.1).
func TestOutIsTheTTYWriter(t *testing.T) {
	tty, _, outR := pipeTTY(t)
	io.WriteString(tty.Out(), "prompt\n")
	tty.out.Close()
	b, _ := io.ReadAll(outR)
	if string(b) != "prompt\n" {
		t.Fatalf("out = %q", b)
	}
}

// Size falls back to 80×24 when the fd is not a terminal.
func TestSizeFallback(t *testing.T) {
	tty, _, _ := pipeTTY(t)
	w, h := tty.Size()
	if w != 80 || h != 24 {
		t.Fatalf("size = %d×%d, want 80×24", w, h)
	}
}
