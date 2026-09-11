package term

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// script returns an event source that replays evs then EOFs.
func script(evs ...event) func() (event, error) {
	i := 0
	return func() (event, error) {
		if i >= len(evs) {
			return event{}, io.EOF
		}
		e := evs[i]
		i++
		return e, nil
	}
}

func TestViewerAltScreenGolden(t *testing.T) {
	var buf bytes.Buffer
	// A representative secret of exactly 52 bytes.
	body := []byte("db: postgres://svc_deploy:wR8-kk2@10.0.4.7:5432/prod")
	err := showViewer(&buf, script(rn('q')), body, ViewerOpts{NoColor: true})
	if err != nil {
		t.Fatal(err)
	}
	want := "\x1b[?1049h\x1b[H\x1b[2J" +
		"  52 bytes | controls escaped\r\n\r\n" +
		"db: postgres://svc_deploy:wR8-kk2@10.0.4.7:5432/prod" +
		"\x1b[24;1H  end | q closes" +
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
	if !strings.Contains(buf.String(), "\x1b[2m1 bytes | controls escaped\x1b[22m") {
		t.Fatalf("missing dim header: %q", buf.String())
	}
}

// Ctrl+C is an interrupt (with rmcup still written). Unrelated keys are
// ignored.
func TestViewerCtrlCAndIgnoredKeys(t *testing.T) {
	var buf bytes.Buffer
	err := showViewer(&buf, script(rn('x'), kd(kindSpace), kd(kindIgnored), kd(kindCtrlC)),
		[]byte("secret"), ViewerOpts{NoColor: true})
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("err = %v, want ErrInterrupted", err)
	}
	if !strings.HasSuffix(buf.String(), "\x1b[?1049l") {
		t.Fatalf("Ctrl+C did not restore the primary screen: %q", buf.String())
	}
}

func TestViewerCancellationAndReadErrorRestorePrimaryScreen(t *testing.T) {
	for name, next := range map[string]func(context.Context) (event, error){
		"canceled": func(ctx context.Context) (event, error) {
			<-ctx.Done()
			return event{}, ctx.Err()
		},
		"read error": func(context.Context) (event, error) {
			return event{}, io.ErrUnexpectedEOF
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if name == "canceled" {
				cancel()
			} else {
				defer cancel()
			}
			var buf bytes.Buffer
			err := showViewerContext(ctx, &buf, next, nil, []byte("secret"), ViewerOpts{NoColor: true}, 80, 24,
				func() error {
					_, err := io.WriteString(&buf, "\x1b[?1049h")
					return err
				},
				func() { _, _ = io.WriteString(&buf, "\x1b[?1049l") })
			want := error(io.ErrUnexpectedEOF)
			if name == "canceled" {
				want = context.Canceled
			}
			if !errors.Is(err, want) {
				t.Fatalf("err = %v, want %v", err, want)
			}
			if !strings.HasSuffix(buf.String(), "\x1b[?1049l") {
				t.Fatalf("primary screen not restored: %q", buf.String())
			}
		})
	}
}

func TestViewerNoAltRendersBeforeReturningCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	err := showViewerContext(ctx, &buf, nil, nil, []byte("claimed secret"), ViewerOpts{NoAlt: true}, 80, 24, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if !strings.Contains(buf.String(), "claimed secret") {
		t.Fatalf("claimed plaintext was not rendered: %q", buf.String())
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

// The header count groups thousands from the actual plaintext length.
func TestViewerByteCountFormatting(t *testing.T) {
	var buf bytes.Buffer
	err := showViewer(&buf, script(rn('q')), bytes.Repeat([]byte("x"), 3214), ViewerOpts{NoColor: true})
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
	if !strings.Contains(buf.String(), "1 bytes | controls escaped") || !strings.Contains(buf.String(), `\x1b`) {
		t.Fatalf("viewer did not preserve source byte count while escaping: %q", buf.String())
	}
}

func TestViewerPagingReachesTailAndReturnsToPreviousPage(t *testing.T) {
	const columns = 49
	first := strings.Repeat("A", columns)
	second := strings.Repeat("B", columns)
	body := []byte(first + second + "TAIL-MARKER")

	var buf bytes.Buffer
	err := showViewerAtSize(&buf, script(kd(kindSpace), rn('b'), kd(kindEnter), kd(kindSpace), rn('q')),
		body, ViewerOpts{NoColor: true}, columns+1, 5)
	if err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if got := strings.Count(output, first); got != 2 {
		t.Fatalf("first page render count = %d, want 2 after back navigation", got)
	}
	if !strings.Contains(output, "TAIL-MARKER") {
		t.Fatalf("final page was unreachable: %q", output)
	}
	if got := strings.Count(output, "\x1b[H\x1b[2J"); got != 5 {
		t.Fatalf("page clear count = %d, want initial frame plus 4 redraws", got)
	}
}

func TestViewerMaximumUnbrokenSecretHasProgressingCompletePageSpans(t *testing.T) {
	body := bytes.Repeat([]byte{'x'}, 65_491)
	const columns, rows = 79, 20
	start, pages := 0, 0
	for start < len(body) {
		end := viewerPageEnd(body, start, columns, rows)
		if end <= start || end > len(body) {
			t.Fatalf("page %d span = [%d:%d] of %d", pages, start, end, len(body))
		}
		start = end
		pages++
	}
	if start != len(body) || pages < 2 {
		t.Fatalf("coverage ended at %d in %d pages, want %d bytes across multiple pages", start, pages, len(body))
	}
}

func TestViewerPageBoundariesDoNotSplitRenderedTokens(t *testing.T) {
	body := append([]byte(strings.Repeat("a", 48)), []byte("é\x1b")...)
	firstEnd := viewerPageEnd(body, 0, 49, 1)
	if firstEnd != 48 {
		t.Fatalf("first page end = %d, want source offset 48 before wide rune", firstEnd)
	}
	secondEnd := viewerPageEnd(body, firstEnd, 49, 1)
	if secondEnd != len(body) {
		t.Fatalf("second page end = %d, want %d", secondEnd, len(body))
	}
	var rendered bytes.Buffer
	if err := writeViewerPage(&rendered, body[firstEnd:secondEnd], 49); err != nil {
		t.Fatal(err)
	}
	if got, want := rendered.String(), `é\x1b`; got != want {
		t.Fatalf("second page = %q, want %q", got, want)
	}

	crlfBody := []byte("a\r\nb")
	if got := viewerPageEnd(crlfBody, 0, 49, 1); got != 3 {
		t.Fatalf("CRLF page end = %d, want 3 after the complete line ending", got)
	}
}

func TestViewerTinyTerminalUsesReachableScrollbackFallback(t *testing.T) {
	var buf bytes.Buffer
	entered := false
	err := showViewerContext(context.Background(), &buf, nil, nil, []byte("all plaintext remains reachable"),
		ViewerOpts{NoColor: true}, 20, 4,
		func() error { entered = true; return nil }, func() {})
	if err != nil {
		t.Fatal(err)
	}
	if entered {
		t.Fatal("tiny terminal entered the alternate screen")
	}
	if got := buf.String(); !strings.Contains(got, "note: plaintext is entering terminal scrollback") ||
		!strings.Contains(got, "all plaintext remains reachable") {
		t.Fatalf("fallback output = %q", got)
	}
}

func TestViewerReflowsAfterShrinkAndTailRemainsReachable(t *testing.T) {
	body := []byte(strings.Repeat("a", 200) + "TAIL")
	events := script(kd(kindResize), kd(kindSpace), kd(kindSpace), kd(kindSpace), kd(kindSpace), rn('q'))
	next := func(context.Context) (event, error) { return events() }
	var buf bytes.Buffer
	enter := func() error {
		_, err := io.WriteString(&buf, "\x1b[?1049h\x1b[H\x1b[2J")
		return err
	}
	leave := func() { _, _ = io.WriteString(&buf, "\x1b[?1049l") }
	err := showViewerContext(context.Background(), &buf, next, func() (int, int) { return 50, 5 },
		body, ViewerOpts{NoColor: true}, 80, 24, enter, leave)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(buf.String(), "TAIL"); got != 2 {
		t.Fatalf("tail render count = %d, want initial frame and reflowed final page; output=%q", got, buf.String())
	}
}

func TestViewerRechecksGeometryWithoutResizeEvent(t *testing.T) {
	body := []byte(strings.Repeat("a", 200) + "TAIL")
	events := script(kd(kindSpace), rn('q'))
	next := func(context.Context) (event, error) { return events() }
	var buf bytes.Buffer
	err := showViewerContext(context.Background(), &buf, next, func() (int, int) { return 50, 5 },
		body, ViewerOpts{NoColor: true}, 80, 24,
		func() error {
			_, err := io.WriteString(&buf, "\x1b[?1049h\x1b[H\x1b[2J")
			return err
		},
		func() { _, _ = io.WriteString(&buf, "\x1b[?1049l") })
	if err != nil {
		t.Fatal(err)
	}
	// Windows has no SIGWINCH. The Space that first reveals changed
	// dimensions must reflow only; it must not also advance past that page.
	if got := strings.Count(buf.String(), "\x1b[H\x1b[2J"); got != 2 {
		t.Fatalf("clear count = %d, want initial frame plus one reflow; output=%q", got, buf.String())
	}
}

func TestViewerResizeBelowFrameRequestsSafeFallback(t *testing.T) {
	events := script(kd(kindResize))
	next := func(context.Context) (event, error) { return events() }
	var buf bytes.Buffer
	err := showViewerContext(context.Background(), &buf, next, func() (int, int) { return 20, 4 },
		[]byte("secret"), ViewerOpts{NoColor: true}, 80, 24,
		func() error { return nil }, func() { _, _ = io.WriteString(&buf, "left") })
	if !errors.Is(err, errViewerFallback) {
		t.Fatalf("err=%v, want errViewerFallback", err)
	}
	if !strings.HasSuffix(buf.String(), "left") {
		t.Fatalf("alternate screen was not left before fallback: %q", buf.String())
	}
}

func TestViewerChromeUsesOnlySingleWidthASCII(t *testing.T) {
	for _, text := range []string{
		viewerHeader(make([]byte, 65_491)),
		viewerFooterMax,
		viewerFooter(false, true),
		viewerFooter(true, false),
		viewerFooter(false, false),
	} {
		for _, r := range text {
			if r < 0x20 || r > 0x7e {
				t.Fatalf("viewer chrome contains non-ASCII rune U+%04X: %q", r, text)
			}
		}
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
		footer := strings.Index(output[start+len(bodyMarker):], "\x1b[24;1H")
		if footer < 0 {
			t.Fatalf("missing trusted footer frame: %q", output)
		}
		gotBody := output[start+len(bodyMarker) : start+len(bodyMarker)+footer]
		wantBody := strings.ReplaceAll(wantLF, "\n", "\r\n")
		if gotBody != wantBody {
			t.Fatalf("body = %q, want %q", gotBody, wantBody)
		}
		if got := strings.Count(output, "\x1b[?1049l"); got != 1 {
			t.Fatalf("alternate-screen exit count = %d, want trusted epilogue only", got)
		}
		if got := strings.Count(output, "\x1b"); got != 5 {
			t.Fatalf("raw ESC count = %d, want prologue, footer-position, and epilogue controls", got)
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

func showViewerAtSize(
	w io.Writer,
	next func() (event, error),
	plaintext []byte,
	o ViewerOpts,
	width, height int,
) error {
	nextContext := func(context.Context) (event, error) { return next() }
	enter := func() error {
		_, err := io.WriteString(w, "\x1b[?1049h\x1b[H\x1b[2J")
		return err
	}
	leave := func() { _, _ = io.WriteString(w, "\x1b[?1049l") }
	return showViewerContext(context.Background(), w, nextContext, nil, plaintext, o, width, height, enter, leave)
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
