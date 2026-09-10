//go:build windows

package secret

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Mlock best-effort pins b's pages via VirtualLock (§12). Failures are
// silently ignored (working-set quota may refuse); see mlock_unix.go for the
// rationale.
func Mlock(b []byte) {
	if len(b) == 0 {
		return
	}
	_ = windows.VirtualLock(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
}
