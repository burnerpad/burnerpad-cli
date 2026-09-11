package term

import (
	"strings"
	"testing"
)

const mintPhrase = "aardvark carrot embroidery hardhat lyrics porcupine suave"

func TestPhraseMachineHasNoFreeformEscape(t *testing.T) {
	m := newMachine(0)
	for i := 0; i < 3; i++ {
		out := m.handle(kd(kindCtrlO))
		if !out.bell {
			t.Fatalf("press %d: bell=%v", i+1, out.bell)
		}
		if !strings.Contains(out.status, "word list") {
			t.Fatalf("press %d: status=%q", i+1, out.status)
		}
	}
}

func TestPlainPhraseEntryTreatsRetiredEscapeAsOffList(t *testing.T) {
	input := "!freeform\n" + mintPhrase + "\n\n"
	var out strings.Builder
	buf, err := readPhrasePlain(strings.NewReader(input), &out, 0)
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
