package term

import (
	"strings"
	"testing"

	"github.com/burnerpad/burnerpad-cli/wordlist"
)

const mintPhrase = "aardvark carrot embroidery hardhat lyrics porcupine suave"

func TestPhraseMachineHasNoFreeformEscape(t *testing.T) {
	m := NewMintingMachine(0)
	for i := 0; i < 3; i++ {
		out := m.Handle(kd(KindCtrlO))
		if m.Mode() != ListLocked || !out.Bell {
			t.Fatalf("press %d: mode=%v bell=%v", i+1, m.Mode(), out.Bell)
		}
		if !strings.Contains(out.Status, "word list") {
			t.Fatalf("press %d: status=%q", i+1, out.Status)
		}
	}
}

func TestPlainPhraseEntryTreatsRetiredEscapeAsOffList(t *testing.T) {
	input := "!freeform\n" + mintPhrase + "\n\n"
	var out strings.Builder
	buf, err := readPhrasePlain(strings.NewReader(input), &out, 0, nil)
	if err != nil {
		t.Fatalf("readPhrasePlain: %v", err)
	}
	defer buf.Wipe()
	if got := string(buf.Bytes()); got != mintPhrase {
		t.Errorf("phrase=%q, want %q", got, mintPhrase)
	}
	if !strings.Contains(out.String(), "a word is not on the Burnerpad word list") {
		t.Fatalf("retired escape was not rejected:\n%s", out.String())
	}
}

func TestValidateMatchesSeedWords(t *testing.T) {
	corpus := []string{
		mintPhrase,
		mintPhrase + " wizardry",
		"",
		" ",
		" " + mintPhrase,
		mintPhrase + " ",
		strings.Replace(mintPhrase, " ", "  ", 1),
		strings.Replace(mintPhrase, "carrot", "hunter2", 1),
		strings.Replace(mintPhrase, "suave", "carrot", 1),
		"Tr0ub4dor&3",
		strings.Join(strings.Fields(mintPhrase)[:6], " "),
		strings.ToUpper(mintPhrase),
	}
	for i := 0; i < 200; i++ {
		corpus = append(corpus, string(wordlist.Phrase()))
	}
	for _, phrase := range corpus {
		canonical, err := wordlist.Canonicalize([]byte(phrase))
		mintable := err == nil && string(canonical) == phrase
		seedable := SeedWords([]byte(phrase), 0) != nil
		if mintable != seedable {
			t.Errorf("disagreement on %q: Validate=%v SeedWords=%v", phrase, mintable, seedable)
		}
	}
}
