//go:build !windows && !darwin

package secret

import "os"

// CreateExclusive creates an owner-only output or recovery file. O_EXCL
// refuses every existing path; there is no overwrite mode.
func CreateExclusive(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}
