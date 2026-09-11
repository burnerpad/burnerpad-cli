// Package secret provides best-effort secret hygiene for a short-lived
// process. This narrows exposure windows but cannot make Go leak-proof; see
// the residual-copy table in SECURITY.md.
package secret

import "runtime"

// Wipe zeroes b in place. clear() compiles to memclr; KeepAlive marks b live
// past the stores so the compiler cannot prove them dead and elide them.
func Wipe(b []byte) {
	if len(b) == 0 {
		return
	}
	clear(b)
	runtime.KeepAlive(b)
}

// Buffer wraps a secret byte slice so that (a) accidental formatting leaks
// nothing — Stringer/GoStringer return "[redacted]" — and (b) the
// owner has one obvious wipe affordance. It deliberately has no Copy: secrets
// are moved, not multiplied.
type Buffer struct {
	b []byte
}

// New takes OWNERSHIP of b: the caller must not retain or reuse the slice.
// Best-effort mlock is applied; failures are silently ignored.
func New(b []byte) *Buffer {
	Mlock(b)
	return &Buffer{b: b}
}

// Bytes returns the underlying secret bytes (nil after Wipe). The slice is
// still owned by the Buffer; callers must not retain it past the Buffer's
// lifetime.
func (s *Buffer) Bytes() []byte {
	if s == nil {
		return nil
	}
	return s.b
}

// Len is the secret's length in bytes (0 for nil or wiped buffers).
func (s *Buffer) Len() int {
	if s == nil {
		return 0
	}
	return len(s.b)
}

// Wipe zeroes and releases the secret. Safe on nil and safe to call twice.
func (s *Buffer) Wipe() {
	if s == nil {
		return
	}
	Wipe(s.b)
	s.b = nil
}

// String implements fmt.Stringer: an accidental %v/%s of a Buffer leaks
// nothing.
func (s *Buffer) String() string { return "[redacted]" }

// GoString implements fmt.GoStringer: %#v leaks nothing either.
func (s *Buffer) GoString() string { return "[redacted]" }
