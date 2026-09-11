package term

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/burnerpad/burnerpad-cli/internal/secret"
	"github.com/burnerpad/burnerpad-cli/wordlist"
)

func readPhrasePlainContext(ctx context.Context, t *TTY, min int) (*secret.Buffer, error) {
	return readPhrasePlainLines(t.out, min, func() ([]byte, error) {
		return t.readLineContext(ctx, wordlist.MaxPhraseBytes)
	})
}

func readPhrasePlainLines(w io.Writer, min int, nextLine func() ([]byte, error)) (*secret.Buffer, error) {
	fmt.Fprintf(w, "Passphrase — one word per line, or the whole phrase on one line.\n")
	fmt.Fprintf(w, "An empty line submits once at least %d words are entered; every word must be on the list.\n", min)
	var committed []string
	for {
		fmt.Fprint(w, plainLabel(len(committed), min))
		line, err := nextLine()
		if err != nil {
			if errors.Is(err, ErrInputTooLong) {
				fmt.Fprintln(w, "input rejected: passphrase line is too long")
				continue
			}
			return nil, err
		}
		switch {
		case len(line) == 0:
			if len(committed) >= min {
				return secret.New(joinPhrase(committed)), nil
			}
			fmt.Fprintf(w, "%d/%d — need at least %d words\n", len(committed), min, min)
			continue
		}
		tokens, issue := parseInteractiveWords(line, committed)
		secret.Wipe(line)
		if issue != interactiveWordsOK {
			fmt.Fprintln(w, interactiveWordsMessage(issue))
			continue
		}
		for _, tok := range tokens {
			committed = append(committed, tok)
			fmt.Fprintf(w, "word %d accepted: %s\n", len(committed), tok)
		}
		if len(committed) >= min {
			fmt.Fprintf(w, "%d words — an empty line submits; keep typing if the phrase was longer\n", len(committed))
		}
	}
}

// plainLabel mirrors promptLabel without the glyphs: spoken-friendly,
// committed-count after the gate (A11).
func plainLabel(n, min int) string {
	if n < min {
		return fmt.Sprintf("word %d/%d: ", n+1, min)
	}
	return fmt.Sprintf("%d words: ", n)
}

// readLine reads one line as fresh bytes with the terminator removed —
// exactly one trailing \n and one preceding \r (§11.6 framing). io.EOF with
// a non-empty final line still yields the line.
func readLine(br *bufio.Reader, limit int) ([]byte, error) {
	line := make([]byte, 0, limit+1)
	overflow := false
	readAny := false
	finish := func() ([]byte, error) {
		if overflow {
			return nil, ErrInputTooLong
		}
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		if len(line) > limit {
			secret.Wipe(line)
			return nil, ErrInputTooLong
		}
		return line, nil
	}
	for {
		b, err := br.ReadByte()
		if err != nil {
			if err == io.EOF && readAny {
				return finish()
			}
			secret.Wipe(line)
			return nil, err
		}
		readAny = true
		if b == '\n' {
			return finish()
		}
		if overflow {
			continue
		}
		if len(line) == limit+1 {
			secret.Wipe(line)
			line = nil
			overflow = true
			continue
		}
		line = append(line, b)
	}
}

// joinPhrase builds the canonical phrase bytes from committed list words.
// Exact-size allocation avoids growth reallocations leaving stale copies.
func joinPhrase(committed []string) []byte {
	size := 0
	for i, w := range committed {
		if i > 0 {
			size++
		}
		size += len(w)
	}
	out := make([]byte, 0, size)
	for i, w := range committed {
		if i > 0 {
			out = append(out, ' ')
		}
		out = append(out, w...)
	}
	return out
}
