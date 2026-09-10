package term

import (
	"bytes"
	"io"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
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
		"  52 bytes · controls escaped · q closes\r\n\r\n" +
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
	if !strings.Contains(buf.String(), "\x1b[2m1 bytes · controls escaped · q closes\x1b[22m") {
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
	want := "note: plaintext is entering terminal scrollback; controls are escaped\n" +
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
	want := "note: plaintext is entering terminal scrollback; controls are escaped\ndone\n"
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

func TestViewerByteCountUsesUnexpandedSourceLength(t *testing.T) {
	var buf bytes.Buffer
	if err := showViewer(&buf, script(rn('q')), []byte{0x1b}, ViewerOpts{NoColor: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "1 bytes · controls escaped") || !strings.Contains(buf.String(), `\x1b`) {
		t.Fatalf("viewer did not preserve source byte count while escaping: %q", buf.String())
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

func TestViewerBodyRendersOnlyGraphicTextAndTrustedLineBreaks(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		newline string
		want    string
	}{
		{name: "graphic UTF-8", body: []byte("café 日本語 👩🏽 e\u0301\u00a0\\"), newline: "\n", want: "café 日本語 👩🏽 e\u0301\u00a0\\"},
		{name: "LF and CRLF", body: []byte("a\nb\r\nc\n"), newline: "\r\n", want: "a\r\nb\r\nc\r\n"},
		{name: "C0 and DEL", body: []byte{0, '\a', '\b', '\t', '\v', '\f', '\r', 0x1b, 0x7f}, newline: "\n", want: `\x00\a\b\t\v\f\r\x1b\x7f`},
		{name: "malformed UTF-8", body: []byte{0x80, 0x9b, 0xc0, 0xaf, 0xe2, 0x82, 0xed, 0xa0, 0x80, 0xff}, newline: "\n", want: `\x80\x9b\xc0\xaf\xe2\x82\xed\xa0\x80\xff`},
		{name: "valid replacement rune", body: []byte("�"), newline: "\n", want: "�"},
		{name: "C1 and format", body: []byte("\u0090\u009b\u009d\u009c\u202e\u2066\u200d\ufeff\u2028\ue000"), newline: "\n", want: `\u0090\u009b\u009d\u009c\u202e\u2066\u200d\ufeff\u2028\ue000`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeViewerBody(&buf, test.body, test.newline); err != nil {
				t.Fatal(err)
			}
			if got := buf.String(); got != test.want {
				t.Fatalf("body = %q, want %q", got, test.want)
			}
		})
	}
}

func TestViewerEscapesAttackerTerminalSequences(t *testing.T) {
	body := []byte("before\x1b]52;c;YQ==\a\n" +
		"\x1b]8;;https://evil.invalid\x1b\\click\x1b]8;;\x1b\\\r\n" +
		"\x1bP1;2|dcs\x1b\\\x1b_apc\x1b\\\x1b[6n\x1b[200~paste\x1b[201~\x1b[?1049lafter")
	wantLF := "before\\x1b]52;c;YQ==\\a\n" +
		"\\x1b]8;;https://evil.invalid\\x1b\\click\\x1b]8;;\\x1b\\\n" +
		"\\x1bP1;2|dcs\\x1b\\\\x1b_apc\\x1b\\\\x1b[6n\\x1b[200~paste\\x1b[201~\\x1b[?1049lafter"

	t.Run("alternate screen", func(t *testing.T) {
		var buf bytes.Buffer
		if err := showViewer(&buf, script(rn('q')), body, ViewerOpts{NoColor: true}); err != nil {
			t.Fatal(err)
		}
		output := buf.String()
		const bodyMarker = "\r\n\r\n"
		start := strings.Index(output, bodyMarker)
		if start < 0 || !strings.HasSuffix(output, "\x1b[?1049l") {
			t.Fatalf("malformed viewer frame: %q", output)
		}
		gotBody := output[start+len(bodyMarker) : len(output)-len("\x1b[?1049l")]
		wantBody := strings.ReplaceAll(wantLF, "\n", "\r\n")
		if gotBody != wantBody {
			t.Fatalf("body = %q, want %q", gotBody, wantBody)
		}
		if got := strings.Count(output, "\x1b[?1049l"); got != 1 {
			t.Fatalf("alternate-screen exit count = %d, want trusted epilogue only", got)
		}
		if got := strings.Count(output, "\x1b"); got != 4 {
			t.Fatalf("raw ESC count = %d, want three trusted prologue controls and one epilogue", got)
		}
		assertViewerTextSafe(t, strings.ReplaceAll(gotBody, "\r\n", "\n"))
	})

	t.Run("plain terminal", func(t *testing.T) {
		var buf bytes.Buffer
		if err := showViewer(&buf, nil, body, ViewerOpts{NoAlt: true}); err != nil {
			t.Fatal(err)
		}
		const note = "note: plaintext is entering terminal scrollback; controls are escaped\n"
		if got, want := buf.String(), note+wantLF+"\n"; got != want {
			t.Fatalf("plain output = %q, want %q", got, want)
		}
		if strings.ContainsRune(buf.String(), '\x1b') {
			t.Fatal("plain viewer emitted an attacker-controlled ESC")
		}
		assertViewerTextSafe(t, strings.TrimPrefix(buf.String(), note))
	})
}

func FuzzViewerBodyIsTerminalSafe(f *testing.F) {
	f.Add([]byte("ordinary UTF-8\nwith lines"))
	f.Add([]byte("\x1b]52;c;YQ==\a\u009b\u202e"))
	allBytes := make([]byte, 256)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}
	f.Add(allBytes)

	f.Fuzz(func(t *testing.T, body []byte) {
		var buf bytes.Buffer
		if err := writeViewerBody(&buf, body, "\n"); err != nil {
			t.Fatal(err)
		}
		assertViewerTextSafe(t, buf.String())
	})
}

func assertViewerTextSafe(t *testing.T, text string) {
	t.Helper()
	if !utf8.ValidString(text) {
		t.Fatalf("viewer rendition is invalid UTF-8: %q", text)
	}
	for _, r := range text {
		if r != '\n' && !strconv.IsGraphic(r) {
			t.Fatalf("viewer rendition contains terminal control U+%04X: %q", r, text)
		}
	}
}
