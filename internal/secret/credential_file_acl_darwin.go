//go:build darwin

package secret

import (
	"encoding/binary"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	darwinFileSecurityMagic       = uint32(0x012cc16d)
	darwinFileSecurityNoACL       = ^uint32(0)
	darwinFileSecurityCountOffset = 36
	darwinFileSecurityHeaderSize  = 44
	darwinFileSecurityACEBytes    = 24
	darwinFileSecurityMaxEntries  = 128
	darwinFileSecurityMaxSize     = darwinFileSecurityHeaderSize + darwinFileSecurityMaxEntries*darwinFileSecurityACEBytes
)

// Darwin ACLs can grant access independently of BSD mode bits. The extended
// stat syscall returns both through the already-open descriptor; unlike a raw
// com.apple.system.Security xattr read, it is available to ordinary users.
func credentialFileStat(fd int) (unix.Stat_t, bool, error) {
	var stat unix.Stat_t
	var fileSecurity [darwinFileSecurityMaxSize]byte
	fileSecuritySize := uintptr(len(fileSecurity))
	//lint:ignore SA1019 CGO-free builds cannot call Apple's acl_get_fd_np wrapper, and x/sys exposes no equivalent.
	fstat64Extended := uintptr(unix.SYS_FSTAT64_EXTENDED)
	_, _, errno := unix.Syscall6(
		fstat64Extended,
		uintptr(fd),
		uintptr(unsafe.Pointer(&stat)),
		uintptr(unsafe.Pointer(&fileSecurity[0])),
		uintptr(unsafe.Pointer(&fileSecuritySize)),
		0,
		0,
	)
	runtime.KeepAlive(&stat)
	runtime.KeepAlive(&fileSecurity)
	runtime.KeepAlive(&fileSecuritySize)
	if errno != 0 {
		return stat, false, errno
	}
	if fileSecuritySize > uintptr(len(fileSecurity)) {
		return stat, false, ErrUnprotectedCredentialFile
	}
	hasACL, err := darwinFileSecurityHasACL(fileSecurity[:fileSecuritySize])
	if err != nil {
		return stat, false, ErrUnprotectedCredentialFile
	}
	return stat, hasACL, nil
}

// fstat64_extended returns the public kauth_filesec ABI in host byte order.
// Every supported Darwin Go architecture is little-endian.
func darwinFileSecurityHasACL(fileSecurity []byte) (bool, error) {
	if len(fileSecurity) == 0 {
		return false, nil
	}
	if len(fileSecurity) < darwinFileSecurityHeaderSize ||
		(len(fileSecurity)-darwinFileSecurityHeaderSize)%darwinFileSecurityACEBytes != 0 {
		return false, unix.EINVAL
	}
	if binary.LittleEndian.Uint32(fileSecurity[:4]) != darwinFileSecurityMagic {
		return false, unix.EINVAL
	}
	entryCount := binary.LittleEndian.Uint32(fileSecurity[darwinFileSecurityCountOffset:])
	if entryCount == darwinFileSecurityNoACL {
		if len(fileSecurity) != darwinFileSecurityHeaderSize {
			return false, unix.EINVAL
		}
		return false, nil
	}
	if entryCount > darwinFileSecurityMaxEntries ||
		len(fileSecurity) != darwinFileSecurityHeaderSize+int(entryCount)*darwinFileSecurityACEBytes {
		return false, unix.EINVAL
	}
	return true, nil
}
