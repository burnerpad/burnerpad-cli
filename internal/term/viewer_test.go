package term

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// script returns a ReadEvent-shaped source that replays evs then EOFs.
func script(evs ...Event) func() (Event, error) {
	i := 0
	return func() (Event, error) {
		if i >= len(evs) {
			return Event{}, io.EOF
		}
		e := evs[i]
		i++
		return e, nil
	}
}

func TestViewerAltScreenGolden(t *testing.T) {
	t.Setenv("TMUX", "")
	var buf bytes.Buffer
	// the §10(c) plaintext: exactly 52 bytes
	body := []byte("db: postgres://svc_deploy:wR8-kk2@10.0.4.7:5432/prod")
	err := showViewer(&buf, script(rn('q')), body, ViewerOpts{NoColor: true})
	if err != nil {
		t.Fatal(err)
	}
	want := "\x1b[?1049h\x1b[H\x1b[2J" +
		"  52 bytes · q closes (nothing enters scrollback)\r\n\r\n" +
		"db: postgres://svc_deploy:wR8-kk2@10.0.4.7:5432/prod" +
		"\x1b[?1049l"
	if got := buf.String(); got != want {
		t.Fatalf("screen = %q\nwant     %q", got, want)
	}
}

func TestViewerHeaderDimUnlessNoColor(t *testing.T) {
	var buf bytes.Buffer
	if err := showViewer(&buf, script(rn('q')), []byte("x"), ViewerOpts{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\x1b[2m1 bytes · q closes (nothing enters scrollback)\x1b[22m") {
		t.Fatalf("missing dim header: %q", buf.String())
	}
}

// Ctrl+C closes cleanly (rmcup written, nil error); the exit itself is the
// caller's job (A8). Unrelated keys are ignored.
func TestViewerCtrlCAndIgnoredKeys(t *testing.T) {
	var buf bytes.Buffer
	err := showViewer(&buf, script(rn('x'), kd(KindSpace), kd(KindIgnored), kd(KindCtrlC)),
		[]byte("secret"), ViewerOpts{NoColor: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(buf.String(), "\x1b[?1049l") {
		t.Fatalf("Ctrl+C did not restore the primary screen: %q", buf.String())
	}
}

// EOF on the event source also closes cleanly — a dead tty must not wedge
// the viewer on the alternate screen.
func TestViewerEOFCloses(t *testing.T) {
	var buf bytes.Buffer
	if err := showViewer(&buf, script(), []byte("x"), ViewerOpts{NoColor: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(buf.String(), "\x1b[?1049l") {
		t.Fatalf("EOF did not restore the primary screen: %q", buf.String())
	}
}

func TestViewerNoAltGolden(t *testing.T) {
	var buf bytes.Buffer
	body := []byte("line one\nline two") // no trailing newline
	err := showViewer(&buf, nil, body, ViewerOpts{NoAlt: true, NoColor: true})
	if err != nil {
		t.Fatal(err)
	}
	want := "note: plaintext is entering terminal scrollback\n" +
		"line one\nline two\n"
	if got := buf.String(); got != want {
		t.Fatalf("plain print = %q, want %q", got, want)
	}
}

func TestViewerNoAltKeepsTrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	if err := showViewer(&buf, nil, []byte("done\n"), ViewerOpts{NoAlt: true}); err != nil {
		t.Fatal(err)
	}
	want := "note: plaintext is entering terminal scrollback\ndone\n"
	if got := buf.String(); got != want {
		t.Fatalf("plain print = %q, want %q", got, want)
	}
}

// The header count honors ByteCount (the caller may show the true plaintext
// size while displaying a rendition) and groups thousands (§8.1: "3,214").
func TestViewerByteCountFormatting(t *testing.T) {
	var buf bytes.Buffer
	err := showViewer(&buf, script(rn('q')), []byte("x"), ViewerOpts{NoColor: true, ByteCount: 3214})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "3,214 bytes") {
		t.Fatalf("missing grouped count: %q", buf.String())
	}
}

func TestGroupDigits(t *testing.T) {
	cases := map[int]string{
		0:       "0",
		52:      "52",
		999:     "999",
		1000:    "1,000",
		3214:    "3,214",
		65507:   "65,507",
		1234567: "1,234,567",
	}
	for n, want := range cases {
		if got := groupDigits(n); got != want {
			t.Fatalf("groupDigits(%d) = %q, want %q", n, got, want)
		}
	}
}

// CRLF already present in the body passes through untranslated; bare LF
// becomes CRLF (raw-mode line-ending transport, not reflow).
func TestViewerBodyCRLFTranslation(t *testing.T) {
	var buf bytes.Buffer
	writeBodyCRLF(&buf, []byte("a\nb\r\nc\n"))
	if got := buf.String(); got != "a\r\nb\r\nc\r\n" {
		t.Fatalf("body = %q", got)
	}
}
