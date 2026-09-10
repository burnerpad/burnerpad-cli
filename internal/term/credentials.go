package term

import (
	"context"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/burnerpad/burnerpad-cli/internal/secret"
)

// ReadLine reads one cooked line from the controlling terminal.
func ReadLine(t *TTY, prompt string) (string, error) {
	return ReadLineContext(context.Background(), t, prompt)
}

// ReadLineContext reads through the TTY-owned pump, so cancellation does not
// abandon a second reader that could steal bytes from a later raw prompt.
func ReadLineContext(ctx context.Context, t *TTY, prompt string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, err := fmt.Fprint(t.out, prompt); err != nil {
		return "", err
	}
	line, err := t.readLineContext(ctx)
	if err != nil {
		return "", err
	}
	return string(line), nil
}

// ReadPassword collects a management token without terminal echo.
func ReadPassword(t *TTY, prompt string) ([]byte, error) {
	return ReadPasswordContext(context.Background(), t, prompt)
}

// ReadPasswordContext establishes and records no-echo raw state before it
// waits on the shared input pump. The state is restored synchronously on every
// return, including cancellation; no hidden x/term reader can outlive it.
func ReadPasswordContext(ctx context.Context, t *TTY, prompt string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	restore, err := t.makePasswordMode()
	if err != nil {
		return nil, err
	}
	restored := false
	defer func() {
		if !restored {
			restore()
		}
	}()
	if _, err := fmt.Fprint(t.out, prompt); err != nil {
		return nil, err
	}
	value, readErr := t.readLineContext(ctx)
	restore()
	restored = true
	if _, err := fmt.Fprintln(t.out); err != nil {
		secret.Wipe(value)
		return nil, err
	}
	if readErr != nil {
		secret.Wipe(value)
		return nil, readErr
	}
	return value, nil
}

// readLineContext reconstructs a line from the sole TTY input pump. In
// cooked mode the OS normally performs editing before these events arrive;
// the editing cases also make the same helper suitable for no-echo raw input.
func (t *TTY) readLineContext(ctx context.Context) ([]byte, error) {
	var line []byte
	for {
		ev, err := t.ReadEventContext(ctx)
		if err != nil {
			if err == io.EOF && len(line) > 0 {
				return line, nil
			}
			secret.Wipe(line)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, ErrInterrupted
		}
		switch ev.Kind {
		case KindRune:
			line = utf8.AppendRune(line, ev.R)
		case KindSpace:
			line = append(line, ' ')
		case KindTab:
			line = append(line, '\t')
		case KindEnter:
			return line, nil
		case KindBackspace:
			line = trimLastRune(line)
		case KindCtrlU:
			secret.Wipe(line)
			line = line[:0]
		case KindCtrlW:
			line = trimLastWord(line)
		case KindCtrlC, KindCtrlD:
			secret.Wipe(line)
			return nil, ErrInterrupted
		case KindPaste:
			line = append(line, ev.Paste...)
			secret.Wipe(ev.Paste)
		case KindCtrlO, KindIgnored:
			// These gestures have no line-input meaning.
		}
	}
}

func trimLastRune(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	_, n := utf8.DecodeLastRune(b)
	if n <= 0 {
		n = 1
	}
	secret.Wipe(b[len(b)-n:])
	return b[:len(b)-n]
}

func trimLastWord(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == ' ' || b[len(b)-1] == '\t') {
		b = trimLastRune(b)
	}
	for len(b) > 0 && b[len(b)-1] != ' ' && b[len(b)-1] != '\t' {
		b = trimLastRune(b)
	}
	return b
}
