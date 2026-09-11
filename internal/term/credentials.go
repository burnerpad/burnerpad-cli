package term

import (
	"context"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/burnerpad/burnerpad-cli/internal/secret"
)

const (
	maxShareURLLineBytes = 4096
	maxTokenLineBytes    = 256
)

// ReadLineContext reads through the TTY-owned pump, so cancellation does not
// abandon a second reader that could steal bytes from a later raw prompt.
func ReadLineContext(ctx context.Context, t *TTY, prompt string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, err := fmt.Fprint(t.out, prompt); err != nil {
		return "", err
	}
	line, err := t.readLineContext(ctx, maxShareURLLineBytes)
	if err != nil {
		return "", err
	}
	return string(line), nil
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
	value, readErr := t.readLineContext(ctx, maxTokenLineBytes)
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
func (t *TTY) readLineContext(ctx context.Context, limit int) ([]byte, error) {
	var line []byte
	overflow := false
	reject := func() {
		secret.Wipe(line)
		line = nil
		overflow = true
	}
	for {
		ev, err := t.readEventContext(ctx)
		if err != nil {
			if err == io.EOF {
				if overflow {
					return nil, ErrInputTooLong
				}
				if len(line) > 0 {
					return line, nil
				}
			}
			secret.Wipe(line)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, ErrInterrupted
		}
		switch ev.Kind {
		case kindRune:
			if !overflow {
				size := utf8.RuneLen(ev.R)
				if size < 0 || size > limit-len(line) {
					reject()
				} else {
					line = utf8.AppendRune(line, ev.R)
				}
			}
		case kindSpace:
			if !overflow {
				if len(line) == limit {
					reject()
				} else {
					line = append(line, ' ')
				}
			}
		case kindTab:
			if !overflow {
				if len(line) == limit {
					reject()
				} else {
					line = append(line, '\t')
				}
			}
		case kindEnter:
			if overflow {
				return nil, ErrInputTooLong
			}
			return line, nil
		case kindBackspace:
			if !overflow {
				line = trimLastRune(line)
			}
		case kindCtrlU:
			if !overflow {
				secret.Wipe(line)
				line = line[:0]
			}
		case kindCtrlW:
			if !overflow {
				line = trimLastWord(line)
			}
		case kindCtrlC, kindCtrlD:
			secret.Wipe(line)
			return nil, ErrInterrupted
		case kindPaste:
			if !overflow {
				if len(ev.Paste) > limit-len(line) {
					reject()
				} else {
					line = append(line, ev.Paste...)
				}
			}
			secret.Wipe(ev.Paste)
		case kindInputTooLong:
			if !overflow {
				reject()
			}
		case kindCtrlO, kindIgnored:
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
