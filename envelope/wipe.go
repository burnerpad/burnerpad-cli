package envelope

import "runtime"

// wipe zeroes b in place. It is the package-private twin of
// internal/secret.Wipe, duplicated so envelope stays
// dependency-free and extractable. clear() compiles to memclr; KeepAlive
// marks b live past the stores so the compiler cannot prove them dead and
// elide them.
func wipe(b []byte) {
	if len(b) == 0 {
		return
	}
	clear(b)
	runtime.KeepAlive(b)
}
