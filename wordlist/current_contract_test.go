package wordlist

import "testing"

func TestCanonicalizeCurrentPhrase(t *testing.T) {
	got, err := Canonicalize([]byte("  AARDVARK\tcarrot\nembroidery hardhat lyrics porcupine suave  "))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if string(got) != "aardvark carrot embroidery hardhat lyrics porcupine suave" {
		t.Fatalf("canonical phrase = %q", got)
	}
	for _, input := range []string{
		"aardvark carrot embroidery hardhat lyrics porcupine",
		"aardvark carrot embroidery hardhat lyrics porcupine carrot",
		"aardvark carrot embroidery hardhat lyrics porcupine notaword",
	} {
		if _, err := Canonicalize([]byte(input)); err == nil {
			t.Fatalf("Canonicalize(%q) succeeded", input)
		}
	}
}

func TestGeneratedCurrentPhrase(t *testing.T) {
	for i := 0; i < 25; i++ {
		phrase := Phrase()
		if _, err := Canonicalize(phrase); err != nil {
			t.Fatalf("generated phrase rejected: %v", err)
		}
	}
}
