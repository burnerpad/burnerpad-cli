//go:build !linux && !darwin && !windows

package secret

// Harden is a no-op on platforms without a hardening API in the dependency
// graph: a hardened process is preferred, a working one is required.
func Harden() {}
