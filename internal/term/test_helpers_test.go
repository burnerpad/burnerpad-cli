package term

import (
	"bufio"
	"context"
	"errors"
	"io"

	"github.com/burnerpad/burnerpad-cli/internal/secret"
	"github.com/burnerpad/burnerpad-cli/wordlist"
)

func showViewer(w io.Writer, next func() (event, error), plaintext []byte, o ViewerOpts) error {
	var nextContext func(context.Context) (event, error)
	if next != nil {
		nextContext = func(context.Context) (event, error) { return next() }
	}
	enter := func() error {
		_, err := io.WriteString(w, "\x1b[?1049h\x1b[H\x1b[2J")
		return err
	}
	leave := func() { _, _ = io.WriteString(w, "\x1b[?1049l") }
	return showViewerContext(context.Background(), w, nextContext, nil, plaintext, o, 80, 24, enter, leave)
}

func readPhrasePlain(r io.Reader, w io.Writer, min int) (*secret.Buffer, error) {
	br := bufio.NewReader(r)
	return readPhrasePlainLines(w, min, func() ([]byte, error) {
		line, err := readLine(br, wordlist.MaxPhraseBytes)
		if err != nil {
			if errors.Is(err, ErrInputTooLong) {
				return nil, err
			}
			return nil, ErrInterrupted
		}
		return line, nil
	})
}
