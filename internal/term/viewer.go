package term

import (
	"context"
	"errors"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/burnerpad/burnerpad-cli/internal/secret"
)

// ViewerOpts configures ShowViewer.
type ViewerOpts struct {
	NoAlt     bool // TERM=dumb / --plain / legacy conhost: no alternate screen
	NoColor   bool
	ByteCount int // header count; 0 means len(plaintext)
}

const (
	viewerFrameRows = 4 // header, separator, separator, footer
	viewerFooterMax = "more · Space/Enter next · b back · q closes"
)

// ShowViewer displays a terminal-safe rendition of plaintext per §8.1: on the
// alternate screen (same buffer discipline as less) to limit primary-scrollback
// exposure, or — NoAlt — a plain print preceded by the scrollback note. Control
// and format characters are visibly escaped.
func ShowViewer(t *TTY, plaintext []byte, o ViewerOpts) error {
	return ShowViewerContext(context.Background(), t, plaintext, o)
}

// ShowViewerContext is ShowViewer with cancellation while the alternate
// screen waits for a key. Ctrl+C is an interrupt, EOF is a clean close, and
// other input errors are preserved for the caller.
func ShowViewerContext(ctx context.Context, t *TTY, plaintext []byte, o ViewerOpts) error {
	width, height := t.Size()
	if !o.NoAlt && !viewerFrameFits(width, height, viewerHeader(plaintext, o)) {
		o.NoAlt = true
	}
	if o.NoAlt {
		return showViewerContext(ctx, t.Out(), nil, plaintext, o, width, height, nil, nil)
	}
	restore, err := t.MakeRaw() // single-key q needs raw mode
	if err != nil {
		o.NoAlt = true
		return showViewerContext(ctx, t.Out(), nil, plaintext, o, width, height, nil, nil)
	}
	defer restore()
	return showViewerContext(ctx, t.Out(), t.ReadEventContext, plaintext, o, width, height,
		t.enterAlternateScreen, t.leaveAlternateScreen)
}

// showViewer is ShowViewer minus the terminal acquisition: writer and event
// source are injected so the screens golden-test against a bytes.Buffer.
func showViewer(w io.Writer, next func() (Event, error), plaintext []byte, o ViewerOpts) error {
	var nextContext func(context.Context) (Event, error)
	if next != nil {
		nextContext = func(context.Context) (Event, error) { return next() }
	}
	enter := func() error {
		_, err := io.WriteString(w, "\x1b[?1049h\x1b[H\x1b[2J")
		return err
	}
	leave := func() { _, _ = io.WriteString(w, "\x1b[?1049l") }
	return showViewerContext(context.Background(), w, nextContext, plaintext, o, 80, 24, enter, leave)
}

func showViewerContext(
	ctx context.Context,
	w io.Writer,
	next func(context.Context) (Event, error),
	plaintext []byte,
	o ViewerOpts,
	width, height int,
	enterAlternate func() error,
	leaveAlternate func(),
) error {
	header := viewerHeader(plaintext, o)
	if !o.NoAlt && !viewerFrameFits(width, height, header) {
		o.NoAlt = true
	}

	if o.NoAlt {
		// §8.1: the honest path when no alternate screen exists.
		if _, err := io.WriteString(w, "note: plaintext is entering terminal scrollback; controls are escaped\n"); err != nil {
			return err
		}
		if err := writeViewerBody(w, plaintext, "\n"); err != nil {
			return err
		}
		if len(plaintext) > 0 && plaintext[len(plaintext)-1] != '\n' {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		return ctx.Err()
	}

	if err := enterAlternate(); err != nil {
		return err
	}
	defer leaveAlternate()

	bodyColumns := width - 1 // never touch the auto-wrap column
	bodyRows := height - viewerFrameRows
	pageStarts := []int{0}
	page := 0
	if err := writeViewerFrame(w, plaintext, pageStarts[page], header, bodyColumns, bodyRows, height, o.NoColor, false); err != nil {
		return err
	}

	for next != nil {
		ev, err := next(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		switch {
		case ev.Kind == KindRune && (ev.R == 'q' || ev.R == 'Q'):
			return nil
		case ev.Kind == KindSpace || ev.Kind == KindEnter:
			end := viewerPageEnd(plaintext, pageStarts[page], bodyColumns, bodyRows)
			if end < len(plaintext) {
				page++
				if page == len(pageStarts) {
					pageStarts = append(pageStarts, end)
				}
				if err := writeViewerFrame(w, plaintext, pageStarts[page], header, bodyColumns, bodyRows, height, o.NoColor, true); err != nil {
					return err
				}
			}
		case ev.Kind == KindRune && (ev.R == 'b' || ev.R == 'B'):
			if page > 0 {
				page--
				if err := writeViewerFrame(w, plaintext, pageStarts[page], header, bodyColumns, bodyRows, height, o.NoColor, true); err != nil {
					return err
				}
			}
		case ev.Kind == KindCtrlC:
			return ErrInterrupted
		case ev.Kind == KindPaste:
			secret.Wipe(ev.Paste) // consumers wipe pastes; the viewer ignores them
		}
	}
	return ctx.Err()
}

func viewerHeader(plaintext []byte, o ViewerOpts) string {
	n := o.ByteCount
	if n == 0 {
		n = len(plaintext)
	}
	return groupDigits(n) + " bytes · controls escaped"
}

func viewerFrameFits(width, height int, header string) bool {
	columns := width - 1
	return height > viewerFrameRows &&
		columns >= 2+utf8.RuneCountInString(header) &&
		columns >= 2+utf8.RuneCountInString(viewerFooterMax)
}

func writeViewerFrame(
	w io.Writer,
	plaintext []byte,
	start int,
	header string,
	columns, rows, height int,
	noColor, clear bool,
) error {
	if clear {
		if _, err := io.WriteString(w, "\x1b[H\x1b[2J"); err != nil {
			return err
		}
	}
	if err := writeViewerLabel(w, header, noColor); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "\r\n\r\n"); err != nil {
		return err
	}
	end := viewerPageEnd(plaintext, start, columns, rows)
	if err := writeViewerPage(w, plaintext[start:end], columns); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "\x1b["+strconv.Itoa(height)+";1H"); err != nil {
		return err
	}
	return writeViewerLabel(w, viewerFooter(start > 0, end < len(plaintext)), noColor)
}

func writeViewerLabel(w io.Writer, text string, noColor bool) error {
	if noColor {
		_, err := io.WriteString(w, "  "+text)
		return err
	}
	_, err := io.WriteString(w, "  \x1b[2m"+text+"\x1b[22m")
	return err
}

func viewerFooter(hasPrevious, hasNext bool) string {
	switch {
	case hasPrevious && hasNext:
		return viewerFooterMax
	case hasNext:
		return "more · Space/Enter next · q closes"
	case hasPrevious:
		return "end · b back · q closes"
	default:
		return "end · q closes"
	}
}

func viewerPageEnd(plaintext []byte, start, columns, rows int) int {
	row, column := 0, 0
	for offset := start; offset < len(plaintext); {
		token := nextViewerToken(plaintext[offset:])
		if token.newline {
			offset += token.sourceBytes
			row++
			column = 0
			if row >= rows {
				return offset
			}
			continue
		}
		if column > 0 && column+token.columns > columns {
			row++
			column = 0
			if row >= rows {
				return offset
			}
		}
		offset += token.sourceBytes
		column += token.columns
		if offset == len(plaintext) {
			return offset
		}
	}
	return len(plaintext)
}

func writeViewerPage(w io.Writer, plaintext []byte, columns int) error {
	column := 0
	for len(plaintext) > 0 {
		token := nextViewerToken(plaintext)
		if token.newline {
			if _, err := io.WriteString(w, "\r\n"); err != nil {
				return err
			}
			column = 0
		} else {
			if column > 0 && column+token.columns > columns {
				if _, err := io.WriteString(w, "\r\n"); err != nil {
					return err
				}
				column = 0
			}
			if _, err := w.Write(token.rendered[:token.renderedBytes]); err != nil {
				return err
			}
			column += token.columns
		}
		plaintext = plaintext[token.sourceBytes:]
	}
	return nil
}

type viewerToken struct {
	sourceBytes   int
	columns       int
	renderedBytes int
	newline       bool
	rendered      [16]byte
}

func nextViewerToken(b []byte) viewerToken {
	var token viewerToken
	if b[0] == '\n' || (b[0] == '\r' && len(b) > 1 && b[1] == '\n') {
		token.sourceBytes = 1
		if b[0] == '\r' {
			token.sourceBytes = 2
		}
		token.newline = true
		return token
	}

	r, size := utf8.DecodeRune(b)
	if r == utf8.RuneError && size == 1 {
		const hex = "0123456789abcdef"
		token.sourceBytes = 1
		token.columns = 4
		token.renderedBytes = 4
		token.rendered[0], token.rendered[1] = '\\', 'x'
		token.rendered[2], token.rendered[3] = hex[b[0]>>4], hex[b[0]&0x0f]
		return token
	}

	token.sourceBytes = size
	if strconv.IsGraphic(r) {
		token.renderedBytes = copy(token.rendered[:], b[:size])
		token.columns = 1
		if r >= utf8.RuneSelf {
			// Without a large Unicode-width table, two columns is the safe
			// upper bound for a graphic scalar. Over-counting combining marks
			// only makes pages more conservative.
			token.columns = 2
		}
		return token
	}

	quoted := strconv.AppendQuoteRuneToGraphic(token.rendered[:0], r)
	copy(token.rendered[:], quoted[1:len(quoted)-1])
	token.renderedBytes = len(quoted) - 2
	token.columns = token.renderedBytes
	return token
}

// writeViewerBody renders untrusted bytes without emitting terminal controls.
// Graphic UTF-8 passes through, LF and CRLF become the caller's trusted line
// ending, and everything else becomes an ASCII Go-style escape.
func writeViewerBody(w io.Writer, b []byte, newline string) error {
	for len(b) > 0 {
		token := nextViewerToken(b)
		if token.newline {
			if _, err := io.WriteString(w, newline); err != nil {
				return err
			}
		} else {
			if _, err := w.Write(token.rendered[:token.renderedBytes]); err != nil {
				return err
			}
		}
		b = b[token.sourceBytes:]
	}
	return nil
}

// groupDigits renders n with thousands separators (§8.1: "3,214 bytes").
func groupDigits(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	lead := len(s) % 3
	if lead > 0 {
		out = append(out, s[:lead]...)
	}
	for i := lead; i < len(s); i += 3 {
		if len(out) > 0 {
			out = append(out, ',')
		}
		out = append(out, s[i:i+3]...)
	}
	return string(out)
}
