//go:build !linux && !darwin && !windows

package secret

// Mlock is a no-op on platforms without a memory-pinning API in the graph.
func Mlock(b []byte) {}
