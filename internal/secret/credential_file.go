package secret

import (
	"errors"
	"os"
)

// ErrUnprotectedCredentialFile means a named credential source did not meet
// the platform's owner-only regular-file policy.
var ErrUnprotectedCredentialFile = errors.New("credential file is not protected")

func credentialPathError(path string, err error) error {
	return &os.PathError{Op: "open", Path: path, Err: err}
}
