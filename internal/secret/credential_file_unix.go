//go:build linux || darwin

package secret

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// OpenCredentialFile opens a named credential without following its final
// path component, then validates the opened descriptor. Ancestor symlinks are
// harmless: type, owner, and permissions are checked on the object actually
// opened, without a path/stat race.
func OpenCredentialFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			err = ErrUnprotectedCredentialFile
		}
		return nil, credentialPathError(path, err)
	}
	stat, hasExtendedACL, err := credentialFileStat(fd)
	if err != nil {
		_ = unix.Close(fd)
		return nil, credentialPathError(path, err)
	}
	if !protectedUnixCredentialStat(uint32(stat.Mode), stat.Uid, uint32(os.Geteuid())) ||
		hasExtendedACL {
		_ = unix.Close(fd)
		return nil, credentialPathError(path, ErrUnprotectedCredentialFile)
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		_ = unix.Close(fd)
		return nil, credentialPathError(path, unix.EBADF)
	}
	return f, nil
}

func protectedUnixCredentialStat(mode, owner, effectiveUser uint32) bool {
	return mode&uint32(unix.S_IFMT) == uint32(unix.S_IFREG) &&
		owner == effectiveUser && mode&0o077 == 0
}
