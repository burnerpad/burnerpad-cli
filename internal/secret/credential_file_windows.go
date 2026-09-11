//go:build windows

package secret

import (
	"errors"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// OpenCredentialFile opens and validates one Windows file handle. Opening the
// reparse point itself prevents the path from being followed; directories,
// reparse points, devices, permissive DACLs, and files owned by another SID
// are rejected before the handle becomes an os.File.
func OpenCredentialFile(path string) (*os.File, error) {
	pathp, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, credentialPathError(path, err)
	}
	handle, err := windows.CreateFile(
		pathp,
		windows.GENERIC_READ|windows.READ_CONTROL,
		0,
		nil, // nil security attributes make the returned handle non-inheritable
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return nil, credentialPathError(path, err)
	}
	reject := func(cause error) (*os.File, error) {
		if closeErr := windows.CloseHandle(handle); closeErr != nil {
			cause = errors.Join(cause, closeErr)
		}
		return nil, credentialPathError(path, cause)
	}

	fileType, err := windows.GetFileType(handle)
	if err != nil {
		return reject(err)
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return reject(err)
	}
	if !protectedWindowsCredentialObject(fileType, info.FileAttributes) {
		return reject(ErrUnprotectedCredentialFile)
	}

	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return reject(err)
	}
	descriptor, err := windows.GetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return reject(err)
	}
	if !protectedWindowsCredentialDescriptor(descriptor, user.User.Sid) {
		return reject(ErrUnprotectedCredentialFile)
	}
	runtime.KeepAlive(descriptor)

	f := os.NewFile(uintptr(handle), path)
	if f == nil {
		return reject(windows.ERROR_INVALID_HANDLE)
	}
	return f, nil
}

func protectedWindowsCredentialObject(fileType, attributes uint32) bool {
	return fileType == windows.FILE_TYPE_DISK &&
		attributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) == 0
}

func protectedWindowsCredentialDescriptor(descriptor *windows.SECURITY_DESCRIPTOR, user *windows.SID) bool {
	if descriptor == nil || user == nil {
		return false
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return false
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.Equals(user) {
		return false
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount == 0 {
		return false
	}
	userAllowed := false
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(dacl, i, &ace) != nil || ace == nil {
			return false
		}
		switch ace.Header.AceType {
		case windows.ACCESS_ALLOWED_ACE_TYPE, windows.ACCESS_DENIED_ACE_TYPE:
			// These are the only two layouts whose SID begins at SidStart.
		default:
			return false
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.IsValid() {
			return false
		}
		if ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE {
			if !sid.Equals(user) || ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
				return false
			}
			userAllowed = true
		}
	}
	return userAllowed
}
