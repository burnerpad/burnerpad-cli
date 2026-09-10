package term

import "github.com/burnerpad/burnerpad-cli/wordlist"

type interactiveWordsIssue uint8

const (
	interactiveWordsOK interactiveWordsIssue = iota
	interactiveWordsInvalid
	interactiveWordsDuplicate
	interactiveWordsTooMany
)

// parseInteractiveWords is the one grammar used by raw paste and plain-line
// phrase entry. It accepts only ASCII whitespace and ASCII case folding, and
// returns strings only after a token has resolved to a public list word.
// Validation is atomic against already committed words.
func parseInteractiveWords(input []byte, committed []string) ([]string, interactiveWordsIssue) {
	seen := make(map[string]struct{}, len(committed))
	for _, word := range committed {
		seen[word] = struct{}{}
	}

	words := wordlist.Words()
	var parsed []string
	for offset := 0; offset < len(input); {
		for offset < len(input) && interactiveASCIISpace(input[offset]) {
			offset++
		}
		if offset == len(input) {
			break
		}
		start := offset
		for offset < len(input) && !interactiveASCIISpace(input[offset]) {
			offset++
		}
		if len(committed)+len(parsed) >= wordlist.MaxPhraseWords {
			return nil, interactiveWordsTooMany
		}
		word, ok := canonicalInteractiveWord(words, input[start:offset])
		if !ok {
			return nil, interactiveWordsInvalid
		}
		if _, duplicate := seen[word]; duplicate {
			return nil, interactiveWordsDuplicate
		}
		seen[word] = struct{}{}
		parsed = append(parsed, word)
	}
	return parsed, interactiveWordsOK
}

func canonicalInteractiveWord(words []string, token []byte) (string, bool) {
	for _, word := range words {
		if len(word) != len(token) {
			continue
		}
		match := true
		for i, c := range token {
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c >= 0x80 || c != word[i] {
				match = false
				break
			}
		}
		if match {
			return word, true
		}
	}
	return "", false
}

func interactiveASCIISpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

func interactiveWordsMessage(issue interactiveWordsIssue) string {
	switch issue {
	case interactiveWordsDuplicate:
		return "a word is repeated"
	case interactiveWordsTooMany:
		return "passphrases contain at most 64 words"
	default:
		return "a word is not on the Burnerpad word list"
	}
}
