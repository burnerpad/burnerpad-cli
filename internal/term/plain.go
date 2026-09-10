package term

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/burnerpad/burnerpad-cli/internal/secret"
	"github.com/burnerpad/burnerpad-cli/wordlist"
)

// suggestMaxDist bounds the closest-word suggestion: past distance 2 the
// nearest list word is noise, not a likely intent (B24 guarantees uniqueness
// only at ≤ 1; ties at 2 resolve in list order).
const suggestMaxDist = 2

// readPhrasePlain is the §7.6 accessibility path as pinned by B24: cooked
// mode, one word (or the whole phrase) per line, list-validated with a
// spoken-friendly echo, no ANSI, no cursor addressing. Line validation is
// atomic like paste (§7.2): a multi-word line commits all tokens or none.
// A valid seed (§7.3 retry) starts committed and is re-echoed with its count
// — a screen-reader user must hear the state; an invalid seed is ignored.
func readPhrasePlain(r io.Reader, w io.Writer, min int, seed []string) (*secret.Buffer, error) {
	br := bufio.NewReader(r)
	fmt.Fprintf(w, "Passphrase — one word per line, or the whole phrase on one line.\n")
	fmt.Fprintf(w, "An empty line submits once at least %d words are entered; every word must be on the list.\n", min)
	words := wordlist.Words()
	var committed []string
	if seedable(words, seed, min) {
		// §7.3: the previous words are kept. Spoken form of the kept-words
		// status, then the standard post-gate reminder — plain mode has no
		// un-commit gesture, so the seed line names every word out loud.
		committed = append(committed, seed...)
		fmt.Fprintf(w, "%d words kept: %s\n", len(committed), strings.Join(committed, " "))
		fmt.Fprintf(w, "%d words — an empty line decrypts; keep typing if the phrase was longer\n", len(committed))
	}
	for {
		fmt.Fprint(w, plainLabel(len(committed), min))
		line, err := readLine(br)
		if err != nil {
			return nil, ErrInterrupted // EOF at a prompt = abort
		}
		s := string(line)
		switch {
		case s == "":
			if len(committed) >= min {
				return secret.New(joinPhrase(committed)), nil
			}
			fmt.Fprintf(w, "%d/%d — need at least %d words\n", len(committed), min, min)
			continue
		}
		tokens := strings.Fields(strings.ToLower(s))
		if ok := validateTokens(w, words, committed, tokens); !ok {
			continue
		}
		for _, tok := range tokens {
			committed = append(committed, tok)
			fmt.Fprintf(w, "word %d accepted: %s\n", len(committed), tok)
		}
		if len(committed) >= min {
			fmt.Fprintf(w, "%d words — an empty line decrypts; keep typing if the phrase was longer\n", len(committed))
		}
	}
}

// validateTokens applies the atomic rule to one line's tokens and echoes the
// first failure (shared not-on-list phrasing with §7.2 paste, per B24).
func validateTokens(w io.Writer, words, committed, tokens []string) bool {
	seen := make(map[string]bool, len(committed)+len(tokens))
	for _, c := range committed {
		seen[c] = true
	}
	for _, tok := range tokens {
		if !inList(words, tok) {
			if cw, d := closestWord(words, tok); d <= suggestMaxDist {
				fmt.Fprintf(w, "%q is not on the word list — closest: %s\n", tok, cw)
			} else {
				fmt.Fprintf(w, "%q is not on the word list\n", tok)
			}
			return false
		}
		if seen[tok] {
			fmt.Fprintf(w, "duplicate word %q\n", tok)
			return false
		}
		seen[tok] = true
	}
	return true
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
func readLine(br *bufio.Reader) ([]byte, error) {
	line, err := br.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return nil, err
	}
	if n := len(line); n > 0 && line[n-1] == '\n' {
		line = line[:n-1]
	}
	if n := len(line); n > 0 && line[n-1] == '\r' {
		line = line[:n-1]
	}
	return line, nil
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
