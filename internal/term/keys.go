// Package term is the terminal layer: the pure list-locked autocomplete
// state machine (ARCHITECTURE.md §7.2 as amended by A9–A12), the pure
// width-aware renderer (§7.2 "Narrow terminals & resize", A10), the plain
// accessibility mode (§7.6, B24), the raw-mode TTY plumbing (§21), the
// alternate-screen viewer (§8.1).
package term

import (
	"unicode/utf8"

	"github.com/burnerpad/burnerpad-cli/internal/secret"
)

// eventKind classifies one decoded input gesture.
type eventKind int

const (
	kindRune eventKind = iota
	kindSpace
	kindTab
	kindEnter
	kindBackspace
	kindCtrlC
	kindCtrlD
	kindCtrlW
	kindCtrlU
	kindCtrlO
	kindPaste
	kindInputTooLong
	kindResize
	kindIgnored
)

// event is one abstract input gesture. The decoder (below) owns byte
// decoding, bracketed-paste framing, and swallowing whole ESC/CSI sequences
// (§7.2 "ESC-initiated sequences" row) — which is why no Escape/Arrow kind
// exists in this vocabulary.
type event struct {
	Kind  eventKind
	R     rune   // kindRune only
	Paste []byte // kindPaste only; may hold secret bytes — the consumer wipes it
}

// pasteEnd is the bracketed-paste terminator (mode 2004, §7.2 paste row).
const pasteEnd = "\x1b[201~"

// maxSeqLen bounds an ESC-initiated sequence, ESC included (§7.2: "bounded
// ≤ 16 bytes"). Anything longer is abandoned as ignored.
const maxSeqLen = 16

// maxPasteBytes bounds the decoder before any consumer-specific validation.
// It covers the largest interactive field (a share URL); phrase and token
// consumers apply their smaller domain limits after decoding.
const maxPasteBytes = 4096

type decodeState int

const (
	dGround decodeState = iota
	dEsc                // ESC seen, dispatch byte pending
	dCSI                // inside ESC [ …
	dSS3                // inside ESC O …
	dUTF8               // inside a multi-byte rune
	dPaste              // inside ESC[200~ … ESC[201~
)

// keyDecoder is the PURE incremental byte-stream → event decoder. It does no
// I/O and keeps no clock: it consumes bytes and reports need-more via
// pending(); the TTY reader owns the 50 ms inter-byte timeout and calls
// flush() when it expires (§7.2 ESC row).
type keyDecoder struct {
	st     decodeState
	seqLen int    // bytes consumed since ESC (bound check)
	csi    []byte // CSI parameter/intermediate bytes
	u8     []byte // partial multi-byte rune
	u8need int    // continuation bytes still expected
	paste  []byte // paste payload collected so far
	pmatch int    // bytes of pasteEnd currently matched
	pover  bool   // payload exceeded maxPasteBytes; scan only to its terminator
	cr     bool   // previous ground byte was CR; suppress its CRLF partner
}

func newKeyDecoder() *keyDecoder { return &keyDecoder{} }

// pending reports whether an ESC/CSI/SS3/UTF-8 sequence is mid-flight — the
// TTY reader applies the inter-byte timeout exactly then. A bracketed paste
// is NOT pending: its terminator is guaranteed by the terminal and the
// payload may arrive in arbitrarily paced chunks.
func (d *keyDecoder) pending() bool {
	switch d.st {
	case dEsc, dCSI, dSS3, dUTF8:
		return true
	}
	return false
}

// flush abandons any mid-flight sequence (inter-byte timeout or EOF): a lone
// ESC, a partial CSI/SS3, a torn rune, or an unterminated paste all become
// one kindIgnored.
func (d *keyDecoder) flush() []event {
	if d.st == dGround {
		return nil
	}
	// An abandoned paste (EOF mid-payload) never reaches a consumer, so the
	// "consumer wipes event.Paste" rule cannot cover it — wipe it here (§12).
	secret.Wipe(d.paste)
	d.reset()
	return []event{{Kind: kindIgnored}}
}

func (d *keyDecoder) reset() {
	d.st = dGround
	d.seqLen = 0
	d.csi = d.csi[:0]
	d.u8 = d.u8[:0]
	d.u8need = 0
	d.paste = nil
	d.pmatch = 0
	d.pover = false
}

// feed consumes one byte and returns zero or more decoded events.
func (d *keyDecoder) feed(b byte) []event {
	switch d.st {
	case dGround:
		return d.ground(b)
	case dEsc:
		d.seqLen++
		switch b {
		case '[':
			d.st = dCSI
			d.csi = d.csi[:0]
			return nil
		case 'O':
			d.st = dSS3
			return nil
		default:
			// Alt-chord (ESC x) or stray dispatch byte: swallow the pair
			// whole — per-byte handling would inject a literal rune.
			d.st = dGround
			return []event{{Kind: kindIgnored}}
		}
	case dSS3:
		// SS3 sequences (F1–F4, keypad) are exactly one byte long.
		d.st = dGround
		d.seqLen = 0
		return []event{{Kind: kindIgnored}}
	case dCSI:
		d.seqLen++
		if d.seqLen > maxSeqLen {
			d.reset()
			return []event{{Kind: kindIgnored}}
		}
		if b >= 0x40 && b <= 0x7e { // final byte
			isPasteStart := b == '~' && string(d.csi) == "200"
			d.reset()
			if isPasteStart {
				d.st = dPaste
				d.paste = make([]byte, 0, maxPasteBytes)
				return nil
			}
			// Arrows, Home/End, Delete, F-keys, mouse, stray ESC[201~ —
			// all ignored whole (this prompt has no cursor movement).
			return []event{{Kind: kindIgnored}}
		}
		d.csi = append(d.csi, b)
		return nil
	case dPaste:
		if b == pasteEnd[d.pmatch] {
			d.pmatch++
			if d.pmatch == len(pasteEnd) {
				p := d.paste
				over := d.pover
				d.reset()
				if over {
					return []event{{Kind: kindInputTooLong}}
				}
				return []event{{Kind: kindPaste, Paste: p}}
			}
			return nil
		}
		// The partial terminator match was payload after all (pastes may
		// contain ESC bytes); replay it, then retry the match at this byte.
		d.appendPaste([]byte(pasteEnd[:d.pmatch]))
		d.pmatch = 0
		if b == pasteEnd[0] {
			d.pmatch = 1
		} else {
			d.appendPaste([]byte{b})
		}
		return nil
	case dUTF8:
		if b&0xc0 != 0x80 { // not a continuation byte: rune torn
			d.u8 = d.u8[:0]
			d.u8need = 0
			d.st = dGround
			return append([]event{{Kind: kindIgnored}}, d.feed(b)...)
		}
		d.u8 = append(d.u8, b)
		d.u8need--
		if d.u8need > 0 {
			return nil
		}
		seq := d.u8
		d.st = dGround
		defer func() { d.u8 = d.u8[:0] }()
		if !utf8.Valid(seq) { // overlong forms, surrogates
			return []event{{Kind: kindIgnored}}
		}
		r, _ := utf8.DecodeRune(seq)
		return []event{{Kind: kindRune, R: r}}
	}
	return nil
}

func (d *keyDecoder) appendPaste(p []byte) {
	if d.pover {
		return
	}
	if len(p) > maxPasteBytes-len(d.paste) {
		secret.Wipe(d.paste)
		d.paste = nil
		d.pover = true
		return
	}
	d.paste = append(d.paste, p...)
}

func (d *keyDecoder) ground(b byte) []event {
	one := func(k eventKind) []event { return []event{{Kind: k}} }
	if d.cr {
		d.cr = false
		if b == '\n' {
			return nil
		}
	}
	switch {
	case b == 0x1b:
		d.st = dEsc
		d.seqLen = 1
		return nil
	case b == 0x03:
		return one(kindCtrlC)
	case b == 0x04:
		return one(kindCtrlD)
	case b == 0x08 || b == 0x7f:
		return one(kindBackspace)
	case b == '\t':
		return one(kindTab)
	case b == '\r':
		d.cr = true
		return one(kindEnter)
	case b == '\n':
		return one(kindEnter)
	case b == 0x0f:
		return one(kindCtrlO)
	case b == 0x15:
		return one(kindCtrlU)
	case b == 0x17:
		return one(kindCtrlW)
	case b == ' ':
		return one(kindSpace)
	case b < 0x20:
		return one(kindIgnored) // remaining C0 controls
	case b < 0x80:
		return []event{{Kind: kindRune, R: rune(b)}}
	case b >= 0xc2 && b <= 0xdf:
		d.startRune(b, 1)
		return nil
	case b >= 0xe0 && b <= 0xef:
		d.startRune(b, 2)
		return nil
	case b >= 0xf0 && b <= 0xf4:
		d.startRune(b, 3)
		return nil
	default:
		// 0x80–0xC1 and 0xF5–0xFF can never start a valid UTF-8 rune.
		return one(kindIgnored)
	}
}

func (d *keyDecoder) startRune(b byte, cont int) {
	d.st = dUTF8
	d.u8 = append(d.u8[:0], b)
	d.u8need = cont
}
