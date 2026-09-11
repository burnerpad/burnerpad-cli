//go:build !linux && !darwin && !windows

package secret

import "os"

// OpenCredentialFile fails closed on platforms outside the supported release
// matrix, where this package has no race-free credential-file policy.
func OpenCredentialFile(path string) (*os.File, error) {
	return nil, credentialPathError(path, ErrUnprotectedCredentialFile)
}
