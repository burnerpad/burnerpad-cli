//go:build darwin

package secret

import (
	"encoding/binary"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	darwinFileSecurityFlagsOffset = darwinFileSecurityCountOffset + 4
	darwinFileSecurityNoInherit   = uint32(1 << 17)
	darwinKauthIdentityNone       = uint32(0xffffff9b)
)

// CreateExclusive atomically creates an owner-only output or recovery file.
// The initial no-inherit ACL policy prevents a parent directory ACL from
// granting access independently of the requested mode bits.
func CreateExclusive(path string) (*os.File, error) {
	pathPointer, err := unix.BytePtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}

	var fileSecurity [darwinFileSecurityHeaderSize]byte
	binary.LittleEndian.PutUint32(fileSecurity[:4], darwinFileSecurityMagic)
	binary.LittleEndian.PutUint32(fileSecurity[darwinFileSecurityCountOffset:], darwinFileSecurityNoACL)
	binary.LittleEndian.PutUint32(fileSecurity[darwinFileSecurityFlagsOffset:], darwinFileSecurityNoInherit)

	flags := uintptr(unix.O_WRONLY | unix.O_CREAT | unix.O_EXCL | unix.O_CLOEXEC | unix.O_NOFOLLOW)
	//lint:ignore SA1019 CGO-free builds cannot call Apple's openx_np wrapper, and x/sys exposes no equivalent.
	openExtended := uintptr(unix.SYS_OPEN_EXTENDED)
	var fd uintptr
	for {
		var errno unix.Errno
		fd, _, errno = unix.Syscall6(
			openExtended,
			uintptr(unsafe.Pointer(pathPointer)),
			flags,
			uintptr(darwinKauthIdentityNone),
			uintptr(darwinKauthIdentityNone),
			uintptr(0o600),
			uintptr(unsafe.Pointer(&fileSecurity[0])),
		)
		runtime.KeepAlive(pathPointer)
		runtime.KeepAlive(&fileSecurity)
		if errno == unix.EINTR {
			continue
		}
		if errno != 0 {
			return nil, &os.PathError{Op: "open", Path: path, Err: errno}
		}
		break
	}

	file := os.NewFile(fd, path)
	if file == nil {
		_ = unix.Close(int(fd))
		return nil, &os.PathError{Op: "open", Path: path, Err: unix.EBADF}
	}
	return file, nil
}
