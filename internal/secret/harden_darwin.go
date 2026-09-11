//go:build darwin

package secret

import "golang.org/x/sys/unix"

// Harden is called as the first statement of main, before any secret exists.
// RLIMIT_CORE=0 stops core files. Deliberately no PT_DENY_ATTACH: it breaks
// legitimate debugging and is trivially bypassed. The error is ignored: a
// hardened process is preferred, a working one is required.
func Harden() {
	_ = unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0})
}
