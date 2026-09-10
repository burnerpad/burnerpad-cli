package envelope

import "testing"

func FuzzDecodeCanonical(f *testing.F) {
	for _, seed := range []string{"", "AA", "AQID_f7_", "A=", "AA\n", "AB"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, encoded string) {
		decoded, err := DecodeCanonical([]byte(encoded))
		if err != nil {
			return
		}
		if got := string(EncodeToBytes(decoded)); got != encoded {
			t.Fatalf("non-canonical input accepted")
		}
	})
}
