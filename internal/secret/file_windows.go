//go:build windows

package secret

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CreateExclusive creates an owner-only plaintext or recovery sink.
// os.OpenFile(…, 0600) on Windows only sets the read-only attribute and lets
// the file inherit the directory's DACL, so the file is created with an
// explicit security descriptor instead: current user owns it and holds full
// control, and nobody else holds anything.
//
// The descriptor is built from SDDL — "O:<sid>D:P(A;;FA;;;<sid>)" — rather
// than assembled ACE-by-ACE with NewSecurityDescriptor/ACLFromEntries: the
// string form is validated as a whole by one OS call and comes back
// self-relative (one blob, no absolute-SD pointer graph to keep alive across
// CreateFile), and the literal SID avoids any account-name lookup or
// localization. "P" (SE_DACL_PROTECTED) blocks inheritable ACEs from the
// parent directory — without it the explicit DACL would be merged with
// whatever the directory grants, defeating owner-only.
func CreateExclusive(path string) (*os.File, error) {
	h, err := createExclusive(path)
	if err != nil {
		// *os.PathError matches os.OpenFile's error shape; ERROR_FILE_EXISTS
		// from CREATE_NEW satisfies errors.Is(err, os.ErrExist) through it.
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}

func createExclusive(path string) (windows.Handle, error) {
	pathp, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	// The pseudo-token needs no CloseHandle.
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return windows.InvalidHandle, err
	}
	sid := tu.User.Sid.String()
	sd, err := windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;;FA;;;" + sid + ")")
	if err != nil {
		return windows.InvalidHandle, err
	}
	sa := &windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(*sa))
	return windows.CreateFile(
		pathp,
		windows.GENERIC_WRITE,
		0, // no sharing: nothing else opens the plaintext while it is written
		sa,
		windows.CREATE_NEW, // the O_EXCL analogue: ERROR_FILE_EXISTS if present
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
}
