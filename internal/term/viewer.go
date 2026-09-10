package term

import (
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
// and format characters are visibly escaped. Ctrl+C also closes cleanly;
// process exit is the caller's job (A8).
func ShowViewer(t *TTY, plaintext []byte, o ViewerOpts) error {
	if o.NoAlt {
		return showViewer(t.Out(), nil, plaintext, o)
	}
	restore, err := t.MakeRaw() // single-key q needs raw mode
	if err != nil {
		o.NoAlt = true
		return showViewer(t.Out(), nil, plaintext, o)
	}
	defer restore()
	return showViewer(t.Out(), t.ReadEvent, plaintext, o)
}

// showViewer is ShowViewer minus the terminal acquisition: writer and event
// source are injected so the screens golden-test against a bytes.Buffer.
func showViewer(w io.Writer, next func() (Event, error), plaintext []byte, o ViewerOpts) error {
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
			_, err := io.WriteString(w, "\n")
			return err
		}
		return nil
	}

	if _, err := io.WriteString(w, "\x1b[?1049h\x1b[H\x1b[2J"); err != nil { // smcup + clear
		return err
	}
	defer func() { _, _ = io.WriteString(w, "\x1b[?1049l") }() // always restore the primary screen
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
		ev, err := next()
		if err != nil {
			break // EOF: nothing left to wait for; close cleanly
		}
		switch {
		case ev.Kind == KindRune && (ev.R == 'q' || ev.R == 'Q'):
			return nil
		case ev.Kind == KindCtrlC:
			return nil // clean close; exit is handled by the caller (A8)
		case ev.Kind == KindPaste:
			secret.Wipe(ev.Paste) // consumers wipe pastes; the viewer ignores them
		}
	}
	return nil
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
