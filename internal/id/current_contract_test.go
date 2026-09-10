package id

import (
	"errors"
	"strings"
	"testing"
)

const canonicalID = "0123456789ABCDEFGHJKMNPQRS"

func TestCurrentContractNormalizesExactlyTwentySixCharacters(t *testing.T) {
	got, err := Normalize("o123-4567-89ab-cdef-ghjk-mnpq-rs")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got != canonicalID {
		t.Fatalf("Normalize = %q, want %q", got, canonicalID)
	}

	for _, raw := range []string{"ABC", strings.Repeat("A", 25), strings.Repeat("A", 27)} {
		if _, err := Normalize(raw); !errors.Is(err, ErrBadID) {
			t.Fatalf("Normalize(%q) error = %v, want ErrBadID", raw, err)
		}
	}
}

func TestCurrentContractSeparatesRevealAndBurnTargets(t *testing.T) {
	reveal, err := ParseShareURL("HTTPS://Example.COM:8443/s/o123-4567-89ab-cdef-ghjk-mnpq-rs")
	if err != nil {
		t.Fatalf("ParseShareURL: %v", err)
	}
	if reveal.Origin != "https://example.com:8443" || reveal.ID != canonicalID {
		t.Fatalf("ParseShareURL = %+v", reveal)
	}

	if _, err := ParseShareURL(canonicalID); !errors.Is(err, ErrBadTarget) {
		t.Fatalf("bare reveal error = %v, want ErrBadTarget", err)
	}
	if _, err := ParseShareURL("https://example.com/s/" + canonicalID + "#fragment"); !errors.Is(err, ErrBadTarget) {
		t.Fatalf("fragment reveal error = %v, want ErrBadTarget", err)
	}

	burn, err := ParseBurnTarget(canonicalID)
	if err != nil {
		t.Fatalf("ParseBurnTarget: %v", err)
	}
	if burn.Origin != "" || burn.ID != canonicalID {
		t.Fatalf("ParseBurnTarget = %+v", burn)
	}
}

func TestCurrentContractRejectsURLSmuggling(t *testing.T) {
	for _, raw := range []string{
		"https://user@example.com/s/" + canonicalID,
		"https://example.com/s/" + canonicalID + "?x=1",
		"https://example.com/s/" + canonicalID + "/more",
		"https://example.com/s/%30" + canonicalID[1:],
		"//example.com/s/" + canonicalID,
		"https://example.com/secret/" + canonicalID,
	} {
		if _, err := ParseShareURL(raw); err == nil {
			t.Errorf("ParseShareURL(%q) succeeded", raw)
		}
	}
}
