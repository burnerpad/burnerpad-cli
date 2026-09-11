package term

import (
	"context"
	"errors"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/burnerpad/burnerpad-cli/internal/secret"
)

// ViewerOpts configures ShowViewerContext.
type ViewerOpts struct {
	NoAlt   bool // TERM=dumb / --plain / legacy conhost: no alternate screen
	NoColor bool
}

const (
	viewerFrameRows = 4 // header, separator, separator, footer
	viewerFooterMax = "more | Space/Enter next | b back | q closes"
)

var errViewerFallback = errors.New("viewer requires scrollback fallback")

// ShowViewerContext displays a terminal-safe rendition of plaintext and
// supports cancellation while the alternate screen waits for a key. Ctrl+C is
// an interrupt, EOF is a clean close, and other input errors are preserved.
func ShowViewerContext(ctx context.Context, t *TTY, plaintext []byte, o ViewerOpts) error {
	width, height := t.size()
	if !o.NoAlt && !viewerFrameFits(width, height, viewerHeader(plaintext)) {
		o.NoAlt = true
	}
	if o.NoAlt {
		return showViewerContext(ctx, t.out, nil, nil, plaintext, o, width, height, nil, nil)
	}
	restore, err := t.makeRaw() // single-key q needs raw mode
	if err != nil {
		o.NoAlt = true
		return showViewerContext(ctx, t.out, nil, nil, plaintext, o, width, height, nil, nil)
	}
	defer restore()
	err = showViewerContext(ctx, t.out, t.readViewerEventContext, t.size, plaintext, o, width, height,
		t.enterAlternateScreen, t.leaveAlternateScreen)
	if !errors.Is(err, errViewerFallback) {
		return err
	}
	// Restore cooked output before the scrollback rendition. Both restore and
	// leaveAlternateScreen are idempotent; their defers remain the error-path net.
	restore()
	o.NoAlt = true
	width, height = t.size()
	return showViewerContext(ctx, t.out, nil, nil, plaintext, o, width, height, nil, nil)
}

func showViewerContext(
	ctx context.Context,
	w io.Writer,
	next func(context.Context) (event, error),
	size func() (int, int),
	plaintext []byte,
	o ViewerOpts,
	width, height int,
	enterAlternate func() error,
	leaveAlternate func(),
) error {
	header := viewerHeader(plaintext)
	if !o.NoAlt && !viewerFrameFits(width, height, header) {
		o.NoAlt = true
	}

	if o.NoAlt {
		// This is the honest path when no alternate screen exists.
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
		// Windows has no SIGWINCH, so do not rely on kindResize as the only
		// geometry trigger. Recheck before every non-closing gesture. If the
		// terminal changed, redraw the page that contained the old source
		// position and consume the gesture: advancing immediately would make
		// part of that newly reflowed page impossible to read.
		if ev.Kind != kindCtrlC && !(ev.Kind == kindRune && (ev.R == 'q' || ev.R == 'Q')) && size != nil {
			newWidth, newHeight := size()
			if newWidth != width || newHeight != height {
				if !viewerFrameFits(newWidth, newHeight, header) {
					return errViewerFallback
				}
				currentStart := pageStarts[page]
				width, height = newWidth, newHeight
				bodyColumns = width - 1
				bodyRows = height - viewerFrameRows
				pageStarts, page = viewerPageStartsAt(plaintext, currentStart, bodyColumns, bodyRows)
				if err := writeViewerFrame(w, plaintext, pageStarts[page], header, bodyColumns, bodyRows, height, o.NoColor, true); err != nil {
					return err
				}
				if ev.Kind == kindPaste {
					secret.Wipe(ev.Paste)
				}
				continue
			}
		}
		switch {
		case ev.Kind == kindRune && (ev.R == 'q' || ev.R == 'Q'):
			return nil
		case ev.Kind == kindSpace || ev.Kind == kindEnter:
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
		case ev.Kind == kindRune && (ev.R == 'b' || ev.R == 'B'):
			if page > 0 {
				page--
				if err := writeViewerFrame(w, plaintext, pageStarts[page], header, bodyColumns, bodyRows, height, o.NoColor, true); err != nil {
					return err
				}
			}
		case ev.Kind == kindResize:
			// A coalesced Unix resize tick whose dimensions were already seen.
			continue
		case ev.Kind == kindCtrlC:
			return ErrInterrupted
		case ev.Kind == kindPaste:
			secret.Wipe(ev.Paste) // consumers wipe pastes; the viewer ignores them
		}
	}
	return ctx.Err()
}

func viewerHeader(plaintext []byte) string {
	n := len(plaintext)
	return groupDigits(n) + " bytes | controls escaped"
}

func viewerFrameFits(width, height int, header string) bool {
	columns := width - 1
	return height > viewerFrameRows &&
		columns >= 2+len(header) &&
		columns >= 2+len(viewerFooterMax)
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
		return "more | Space/Enter next | q closes"
	case hasPrevious:
		return "end | b back | q closes"
	default:
		return "end | q closes"
	}
}

func viewerPageStartsAt(plaintext []byte, target, columns, rows int) ([]int, int) {
	starts := []int{0}
	for {
		page := len(starts) - 1
		end := viewerPageEnd(plaintext, starts[page], columns, rows)
		if end >= len(plaintext) || end > target {
			return starts, page
		}
		if end <= starts[page] {
			return starts, page
		}
		starts = append(starts, end)
	}
}

// readViewerEventContext is the viewer's sole input wait. Unlike ordinary
// line reads, it surfaces coalesced terminal resize ticks as events so a page
// is never rendered with stale dimensions.
func (t *TTY) readViewerEventContext(ctx context.Context) (event, error) {
	if err := ctx.Err(); err != nil {
		return event{}, err
	}
	t.startPump()
	select {
	case <-ctx.Done():
		return event{}, ctx.Err()
	case <-t.winch:
		return event{Kind: kindResize}, nil
	case ev, ok := <-t.events:
		if !ok {
			if t.readErr != nil && t.readErr != io.EOF {
				return event{}, t.readErr
			}
			return event{}, io.EOF
		}
		return ev, nil
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

// groupDigits renders n with thousands separators, as in "3,214 bytes".
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
