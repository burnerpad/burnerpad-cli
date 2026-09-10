package wordlist

import (
	"errors"
	"sort"
	"unicode/utf8"
)

const (
	MaxPhraseWords = 64
	MaxPhraseBytes = 1024
)

var ErrInvalidPhrase = errors.New("passphrase must contain 7 to 64 distinct Burnerpad words")

// Canonicalize accepts ASCII whitespace and returns the lowercase,
// single-space phrase shared by the browser and CLI clients.
func Canonicalize(input []byte) ([]byte, error) {
	if len(input) == 0 || len(input) > MaxPhraseBytes || !utf8.Valid(input) {
		return nil, ErrInvalidPhrase
	}
	var tokens [][]byte
	for i := 0; i < len(input); {
		for i < len(input) && asciiSpace(input[i]) {
			i++
		}
		if i == len(input) {
			break
		}
		start := i
		for i < len(input) && !asciiSpace(input[i]) {
			i++
		}
		tokens = append(tokens, input[start:i])
		if len(tokens) > MaxPhraseWords {
			return nil, ErrInvalidPhrase
		}
	}
	if len(tokens) < PhraseWords || len(tokens) > MaxPhraseWords {
		return nil, ErrInvalidPhrase
	}
	words := Words()
	seen := make(map[int]struct{}, len(tokens))
	size := len(tokens) - 1
	canonical := make([][]byte, len(tokens))
	for i, token := range tokens {
		lower := make([]byte, len(token))
		for j, c := range token {
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			lower[j] = c
		}
		idx := sort.SearchStrings(words, string(lower))
		if idx == len(words) || words[idx] != string(lower) {
			return nil, ErrInvalidPhrase
		}
		if _, exists := seen[idx]; exists {
			return nil, ErrInvalidPhrase
		}
		seen[idx] = struct{}{}
		canonical[i] = lower
		size += len(lower)
	}
	out := make([]byte, 0, size)
	for i, token := range canonical {
		if i != 0 {
			out = append(out, ' ')
		}
		out = append(out, token...)
	}
	return out, nil
}

func asciiSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}
