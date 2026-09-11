//go:build linux || darwin

package secret

import "golang.org/x/sys/unix"

// Mlock best-effort pins b's pages so they cannot be swapped. Failures
// are silently ignored: the default RLIMIT_MEMLOCK far exceeds the ≤ 64 KiB
// working set, but unprivileged containers may refuse — a hardened process is
// preferred, a working one is required. Safe because Go's heap is non-moving;
// all secret buffers are heap-allocated slices escaping package boundaries.
func Mlock(b []byte) {
	if len(b) == 0 {
		return
	}
	_ = unix.Mlock(b)
}
