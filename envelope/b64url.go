package envelope

import (
	"bytes"
	"encoding/base64"
)

// DecodeCanonical decodes strict CANONICAL base64url (SPEC §3): alphabet
// [A-Za-z0-9_-] only, no padding, no whitespace, no impossible length
// (len%4 == 1), no non-canonical trailing bits. Every failure is
// reject_bad_encoding — never a raw base64.CorruptInputError.
//
// Go pitfalls this function exists to close (called out in SPEC §3 itself):
//   - base64.RawURLEncoding.Decode silently SKIPS embedded '\r' and '\n', and
//     accepts dirty trailing bits ("AB" decodes although only "AA" re-encodes).
//   - .Strict() fixes ONLY the trailing-bit leniency; per the encoding/base64
//     docs, CR/LF are STILL ignored in strict mode. The alphabet pre-scan
//     closes that hole up front. (The re-encode-and-compare below would also
//     catch it — re-encoding never emits CR/LF — so the pre-scan is defense
//     in depth and the cheapest, clearest reject, not the sole guard.)
//   - len%4 == 1 cannot be produced by any encoder; today's stdlib happens to
//     reject it, but we check explicitly so the property is enforced here,
//     not inherited from an implementation detail.
//
// The final re-encode-and-compare is the SPEC's own conformance oracle: it
// admits exactly ONE encoding per byte string, making this decoder at least
// as strict as any conformant one even if stdlib leniencies change.
//
// Takes and returns []byte so decoded ciphertext and capabilities can be
// wiped by their owners.
func DecodeCanonical(s []byte) ([]byte, error) {
	if len(s)%4 == 1 {
		return nil, ErrBadEncoding
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return nil, ErrBadEncoding // rejects '=', '+', '/', ' ', '\r', '\n', UTF-8 junk
		}
	}
	out := make([]byte, base64.RawURLEncoding.DecodedLen(len(s)))
	n, err := base64.RawURLEncoding.Strict().Decode(out, s)
	if err != nil {
		wipe(out)
		return nil, ErrBadEncoding // trailing-bit dirt ("AB", "AAB") lands here…
	}
	out = out[:n]
	re := make([]byte, base64.RawURLEncoding.EncodedLen(n))
	base64.RawURLEncoding.Encode(re, out)
	eq := bytes.Equal(re, s)
	wipe(re) // re may be a copy of secret ciphertext or capability material
	if !eq {
		wipe(out)
		return nil, ErrBadEncoding // …and here — defense in depth
	}
	return out, nil
}

// EncodeToBytes returns canonical unpadded base64url as wipeable bytes.
func EncodeToBytes(b []byte) []byte {
	out := make([]byte, base64.RawURLEncoding.EncodedLen(len(b)))
	base64.RawURLEncoding.Encode(out, b)
	return out
}
