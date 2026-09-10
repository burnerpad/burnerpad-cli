//go:build windows

package secret

// Harden is a deliberate no-op on Windows (§12): there are no core files by
// default, and opting out of WER crash dumps would require registry writes
// (Windows Error Reporting\LocalDumps), which the CLI refuses to make. This
// is a documented residual gap — a machine configured to collect user-mode
// dumps can still capture one.
func Harden() {}
