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
	if o.NoAlt {
		return showViewerContext(ctx, t.Out(), nil, plaintext, o, nil, nil)
	}
	restore, err := t.MakeRaw() // single-key q needs raw mode
	if err != nil {
		o.NoAlt = true
		return showViewerContext(ctx, t.Out(), nil, plaintext, o, nil, nil)
	}
	defer restore()
	return showViewerContext(ctx, t.Out(), t.ReadEventContext, plaintext, o,
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
	return showViewerContext(context.Background(), w, nextContext, plaintext, o, enter, leave)
}

func showViewerContext(
	ctx context.Context,
	w io.Writer,
	next func(context.Context) (Event, error),
	plaintext []byte,
	o ViewerOpts,
	enterAlternate func() error,
	leaveAlternate func(),
) error {
	n := o.ByteCount
	if n == 0 {
		n = len(plaintext)
	}
	header := groupDigits(n) + " bytes · controls escaped · q closes"

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
	if o.NoColor {
		if _, err := io.WriteString(w, "  "+header+"\r\n\r\n"); err != nil {
			return err
		}
	} else {
		if _, err := io.WriteString(w, "  \x1b[2m"+header+"\x1b[22m\r\n\r\n"); err != nil {
			return err
		}
	}
	// Raw mode has no output post-processing, so logical line breaks must
	// become CRLF on the wire or lines stair-step.
	if err := writeViewerBody(w, plaintext, "\r\n"); err != nil {
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
		case ev.Kind == KindCtrlC:
			return ErrInterrupted
		case ev.Kind == KindPaste:
			secret.Wipe(ev.Paste) // consumers wipe pastes; the viewer ignores them
		}
	}
	return ctx.Err()
}

// writeViewerBody renders untrusted bytes without emitting terminal controls.
// Graphic UTF-8 passes through, LF and CRLF become the caller's trusted line
// ending, and everything else becomes an ASCII Go-style escape.
func writeViewerBody(w io.Writer, b []byte, newline string) error {
	var escapeBuf [12]byte
	for len(b) > 0 {
		graphicBytes := 0
		for graphicBytes < len(b) {
			if b[graphicBytes] == '\n' ||
				(b[graphicBytes] == '\r' && graphicBytes+1 < len(b) && b[graphicBytes+1] == '\n') {
				break
			}
			r, size := utf8.DecodeRune(b[graphicBytes:])
			if (r == utf8.RuneError && size == 1) || !strconv.IsGraphic(r) {
				break
			}
			graphicBytes += size
		}
		if graphicBytes > 0 {
			if _, err := w.Write(b[:graphicBytes]); err != nil {
				return err
			}
			b = b[graphicBytes:]
			continue
		}

		if b[0] == '\n' || (b[0] == '\r' && len(b) > 1 && b[1] == '\n') {
			consumed := 1
			if b[0] == '\r' {
				consumed = 2
			}
			if _, err := io.WriteString(w, newline); err != nil {
				return err
			}
			b = b[consumed:]
			continue
		}
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size == 1 {
			const hex = "0123456789abcdef"
			escapeBuf[0], escapeBuf[1] = '\\', 'x'
			escapeBuf[2], escapeBuf[3] = hex[b[0]>>4], hex[b[0]&0x0f]
			if _, err := w.Write(escapeBuf[:4]); err != nil {
				return err
			}
			b = b[1:]
		} else {
			quoted := strconv.AppendQuoteRuneToGraphic(escapeBuf[:0], r)
			if _, err := w.Write(quoted[1 : len(quoted)-1]); err != nil {
				return err
			}
			b = b[size:]
		}
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
