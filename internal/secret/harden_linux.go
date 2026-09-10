//go:build linux

package secret

import "golang.org/x/sys/unix"

// Harden is called as the first statement of main, before any secret exists
// (§12). RLIMIT_CORE=0 stops core files; PR_SET_DUMPABLE=0 additionally makes
// the process non-ptrace-attachable and its /proc/pid/mem unreadable by
// same-uid processes. Every error is ignored: a hardened process is
// preferred, a working one is required.
func Harden() {
	_ = unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0})
	_ = unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0)
}
