package term

import (
	"encoding/base64"
	"io"
	"os"

	"github.com/burnerpad/burnerpad-cli/internal/secret"
)

// OSC52Copy writes the terminal clipboard-set sequence
// ESC ] 52 ; c ; <base64 data> BEL, wrapped in tmux passthrough
// (ESC Ptmux; … ESC \ with ESC-doubling) when $TMUX is set (§8.1, B7).
// Success cannot be verified — terminals without OSC 52 ignore it (A7).
// data may be secret; every intermediate buffer is wiped. (base64 ENCODING
// only — the §11.4 M1 gate governs decoding.)
func OSC52Copy(w io.Writer, data []byte) error { return writeOSC52(w, data) }

// OSC52Clear overwrites the clipboard with an empty payload ("52;c;") —
// the timed-clear half of --clip (§8.1).
func OSC52Clear(w io.Writer) error { return writeOSC52(w, nil) }

func writeOSC52(w io.Writer, data []byte) error {
	enc := base64.StdEncoding
	b64 := make([]byte, enc.EncodedLen(len(data)))
	enc.Encode(b64, data)
	seq := make([]byte, 0, len(b64)+8)
	seq = append(seq, "\x1b]52;c;"...)
	seq = append(seq, b64...)
	seq = append(seq, '\a')
	defer secret.Wipe(b64)
	defer secret.Wipe(seq)

	if os.Getenv("TMUX") == "" {
		_, err := w.Write(seq)
		return err
	}
	wrapped := make([]byte, 0, 2*len(seq)+9)
	wrapped = append(wrapped, "\x1bPtmux;"...)
	for _, c := range seq {
		if c == 0x1b {
			wrapped = append(wrapped, 0x1b) // tmux passthrough doubles ESC
		}
		wrapped = append(wrapped, c)
	}
	wrapped = append(wrapped, 0x1b, '\\')
	defer secret.Wipe(wrapped)
	_, err := w.Write(wrapped)
	return err
}
