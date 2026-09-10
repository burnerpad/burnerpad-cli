package term

import (
	"strings"
	"testing"

	"github.com/burnerpad/burnerpad-cli/wordlist"
)

func TestInteractiveWordsMatchCanonicalGrammar(t *testing.T) {
	words := wordlist.Words()[:7]
	for _, separator := range []byte{' ', '\t', '\n', '\r', '\v', '\f'} {
		input := []byte(strings.ToUpper(strings.Join(words, string(separator))))
		parsed, issue := parseInteractiveWords(input, nil)
		if issue != interactiveWordsOK {
			t.Fatalf("separator %#x: issue = %v", separator, issue)
		}
		canonical, err := wordlist.Canonicalize(input)
		if err != nil {
			t.Fatalf("separator %#x: Canonicalize: %v", separator, err)
		}
		if got := strings.Join(parsed, " "); got != string(canonical) {
			t.Fatalf("separator %#x: interactive = %q, canonical = %q", separator, got, canonical)
		}
	}
}

func TestInteractiveWordsRejectNonASCIIAndMalformedInput(t *testing.T) {
	for name, input := range map[string][]byte{
		"non-breaking space": []byte("acrobat\u00a0cufflink"),
		"em space":           []byte("acrobat\u2003cufflink"),
		"case lookalike":     []byte("Ａcrobat cufflink"),
		"malformed UTF-8":    {'a', 'c', 'r', 0xff},
	} {
		if parsed, issue := parseInteractiveWords(input, nil); issue != interactiveWordsInvalid || parsed != nil {
			t.Fatalf("%s: parsed=%v issue=%v, want invalid", name, parsed, issue)
		}
	}
}

func TestInteractiveWordsRejectOversizedPasteBeforeParsing(t *testing.T) {
	input := []byte(strings.Repeat(" ", wordlist.MaxPhraseBytes+1))
	if parsed, issue := parseInteractiveWords(input, nil); issue != interactiveWordsTooLong || parsed != nil {
		t.Fatalf("parsed=%v issue=%v, want too long", parsed, issue)
	}
	m := NewMachine(0)
	m.Handle(rn('t'))
	m.Handle(rn('u'))
	out := m.Handle(Event{Kind: KindInputTooLong})
	if !out.Bell || out.Buf != "tu" || len(out.Committed) != 0 || out.Status != "paste rejected: input is too long" {
		t.Fatalf("overflow event mutated machine: %+v", out)
	}
}

func TestInteractiveWordsEnforceDistinctCombined64WordLimit(t *testing.T) {
	words := wordlist.Words()
	first64 := strings.Join(words[:wordlist.MaxPhraseWords], " ")
	parsed, issue := parseInteractiveWords([]byte(first64), nil)
	if issue != interactiveWordsOK || len(parsed) != wordlist.MaxPhraseWords {
		t.Fatalf("64 words: len=%d issue=%v", len(parsed), issue)
	}
	if _, issue := parseInteractiveWords([]byte(strings.Join(words[:wordlist.MaxPhraseWords+1], " ")), nil); issue != interactiveWordsTooMany {
		t.Fatalf("65 words: issue=%v, want too many", issue)
	}
	if _, issue := parseInteractiveWords([]byte(words[1]), []string{words[0], words[1]}); issue != interactiveWordsDuplicate {
		t.Fatalf("combined duplicate: issue=%v, want duplicate", issue)
	}
	if _, issue := parseInteractiveWords([]byte(words[wordlist.MaxPhraseWords]), words[:wordlist.MaxPhraseWords]); issue != interactiveWordsTooMany {
		t.Fatalf("combined 65th word: issue=%v, want too many", issue)
	}
}

func TestMachineRejectsWord65AtomicallyForPasteAndTypedCommit(t *testing.T) {
	words := wordlist.Words()
	first64 := strings.Join(words[:wordlist.MaxPhraseWords], " ")

	pasted := NewMachine(0)
	out := pasted.Handle(paste(first64))
	if out.Bell || len(out.Committed) != wordlist.MaxPhraseWords {
		t.Fatalf("64-word paste: bell=%v committed=%d", out.Bell, len(out.Committed))
	}
	out = pasted.Handle(paste(words[wordlist.MaxPhraseWords]))
	if !out.Bell || len(out.Committed) != wordlist.MaxPhraseWords || out.Status != "paste rejected: passphrases contain at most 64 words" {
		t.Fatalf("65th pasted word: %+v", out)
	}

	typed := NewMachine(0)
	typed.Handle(paste(first64))
	for _, r := range words[wordlist.MaxPhraseWords] {
		out = typed.Handle(rn(r))
		if out.Bell {
			t.Fatalf("typing word 65 rejected before commit: %+v", out)
		}
	}
	out = typed.Handle(kd(KindSpace))
	if !out.Bell || len(out.Committed) != wordlist.MaxPhraseWords || out.Buf != words[wordlist.MaxPhraseWords] ||
		out.Status != "passphrases contain at most 64 words" {
		t.Fatalf("65th typed commit: %+v", out)
	}
}
