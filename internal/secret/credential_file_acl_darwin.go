//go:build darwin

package secret

import (
	"errors"

	"golang.org/x/sys/unix"
)

const darwinFileSecurityAttribute = "com.apple.system.Security"

// Darwin ACLs can grant access independently of BSD mode bits. XNU stores
// that file-security record in this extended attribute; querying it through
// the already-open descriptor keeps the check race-free. Any record is
// rejected rather than parsing an evolving kernel ACL format here.
func credentialFileHasExtendedACL(fd int) (bool, error) {
	_, err := unix.Fgetxattr(fd, darwinFileSecurityAttribute, nil)
	if errors.Is(err, unix.ENOATTR) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
