package id

import "testing"

func FuzzNormalizeID(f *testing.F) {
	for _, seed := range []string{
		"0123456789ABCDEFGHJKMNPQRS",
		"o123-4567-89ab-cdef-ghjk-mnpq-rs",
		"0\u013123456789ABCDEFGHJKMNPQRS",
		"0123456789ABCDEFGHJKMNPQR\u017f",
		"",
		"../bad",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		normalized, err := Normalize(input)
		for i := 0; i < len(input); i++ {
			if input[i] >= 0x80 {
				if err == nil {
					t.Fatal("non-ASCII identifier normalized successfully")
				}
				return
			}
		}
		if err != nil {
			return
		}
		if len(normalized) != 26 {
			t.Fatalf("normalized length = %d", len(normalized))
		}
		again, err := Normalize(normalized)
		if err != nil || again != normalized {
			t.Fatalf("normalization is not idempotent")
		}
	})
}

func FuzzParseShareURL(f *testing.F) {
	for _, seed := range []string{"https://burnerpad.io/s/0123456789ABCDEFGHJKMNPQRS", "https://user@example.com/s/0123456789ABCDEFGHJKMNPQRS", "0123456789ABCDEFGHJKMNPQRS"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		target, err := ParseShareURL(input)
		if err != nil {
			return
		}
		if target.Origin == "" || len(target.ID) != 26 {
			t.Fatalf("invalid parsed target")
		}
	})
}
