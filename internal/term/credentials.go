package term

import (
	"fmt"
	"io"

	"github.com/burnerpad/burnerpad-cli/internal/secret"
	xterm "golang.org/x/term"
)

// ReadLine reads one cooked line from the controlling terminal without
// buffering beyond its newline, so a later raw-mode prompt cannot lose input.
func ReadLine(t *TTY, prompt string) (string, error) {
	if _, err := fmt.Fprint(t.out, prompt); err != nil {
		return "", err
	}
	var line []byte
	var one [1]byte
	for {
		n, err := t.in.Read(one[:])
		if n == 1 {
			if one[0] == '\n' {
				break
			}
			if one[0] != '\r' {
				line = append(line, one[0])
			}
		}
		if err != nil {
			if err == io.EOF && len(line) > 0 {
				break
			}
			return "", ErrInterrupted
		}
	}
	return string(line), nil
}

// ReadPassword collects a management token without terminal echo.
func ReadPassword(t *TTY, prompt string) ([]byte, error) {
	if _, err := fmt.Fprint(t.out, prompt); err != nil {
		return nil, err
	}
	value, err := xterm.ReadPassword(int(t.in.Fd()))
	if err != nil {
		return nil, ErrInterrupted
	}
	if _, err := fmt.Fprintln(t.out); err != nil {
		secret.Wipe(value)
		return nil, err
	}
	return value, nil
}
