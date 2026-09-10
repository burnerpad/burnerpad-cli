package term

import (
	"io"
	"strconv"

	"github.com/burnerpad/burnerpad-cli/internal/secret"
)

// ViewerOpts configures ShowViewer.
type ViewerOpts struct {
	NoAlt     bool // TERM=dumb / --plain / legacy conhost: no alternate screen
	NoColor   bool
	ByteCount int // header count; 0 means len(plaintext)
}

// ShowViewer displays plaintext per §8.1: on the alternate screen (same
// buffer discipline as less) with q closing so zero secret bytes reach
// scrollback, or — NoAlt — a plain print preceded by the scrollback note.
// Ctrl+C also closes cleanly; process exit is the caller's job (A8).
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
	header := groupDigits(n) + " bytes · q closes (nothing enters scrollback)"

	if o.NoAlt {
		// §8.1: the honest path when no alternate screen exists.
		if _, err := io.WriteString(w, "note: plaintext is entering terminal scrollback\n"); err != nil {
			return err
		}
		if _, err := w.Write(plaintext); err != nil {
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
	// Body content is written without reflow; raw mode has no output
	// post-processing, so bare LF must become CRLF on the wire or lines
	// stair-step. That is line-ending transport, not reformatting.
	if err := writeBodyCRLF(w, plaintext); err != nil {
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

// writeBodyCRLF writes b translating bare LF to CRLF (raw-mode display);
// existing CRLF pairs pass through untouched.
func writeBodyCRLF(w io.Writer, b []byte) error {
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] != '\n' {
			continue
		}
		if i > 0 && b[i-1] == '\r' {
			continue
		}
		if _, err := w.Write(b[start:i]); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\r\n"); err != nil {
			return err
		}
		start = i + 1
	}
	_, err := w.Write(b[start:])
	return err
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
