//go:build windows

package secret

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func TestOpenCredentialFileAcceptsCreateExclusiveFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential")
	created, err := CreateExclusive(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := created.Write([]byte("credential")); err != nil {
		t.Fatal(err)
	}
	if err := created.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := OpenCredentialFile(path)
	if err != nil {
		t.Fatalf("OpenCredentialFile: %v", err)
	}
	got, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || string(got) != "credential" {
		t.Fatalf("content=%q read=%v close=%v", got, readErr, closeErr)
	}
}

func TestOpenCredentialFileRejectsUnsafeWindowsObjects(t *testing.T) {
	t.Run("directory", func(t *testing.T) {
		assertUnprotectedCredentialFile(t, t.TempDir())
	})
	t.Run("reparse point", func(t *testing.T) {
		directory := t.TempDir()
		target := filepath.Join(directory, "target")
		created, err := CreateExclusive(target)
		if err != nil {
			t.Fatal(err)
		}
		if err := created.Close(); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(directory, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("creating a Windows symlink requires OS permission: %v", err)
		}
		assertUnprotectedCredentialFile(t, link)
	})
	t.Run("other principal allowed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "credential")
		created, err := CreateExclusive(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := created.Close(); err != nil {
			t.Fatal(err)
		}
		user := currentWindowsUserSID(t)
		descriptor := windowsDescriptor(t, "O:"+user.String()+"D:P(A;;FA;;;"+user.String()+")(A;;GR;;;WD)")
		dacl, _, err := descriptor.DACL()
		if err != nil {
			t.Fatal(err)
		}
		if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
			nil, nil, dacl, nil); err != nil {
			t.Fatal(err)
		}
		runtime.KeepAlive(descriptor)
		assertUnprotectedCredentialFile(t, path)
	})
	if _, err := OpenCredentialFile(filepath.Join(t.TempDir(), "missing")); err == nil || errors.Is(err, ErrUnprotectedCredentialFile) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing-file error=%v", err)
	}
}

func TestProtectedWindowsCredentialDescriptor(t *testing.T) {
	user := currentWindowsUserSID(t)
	uid := user.String()
	for _, test := range []struct {
		name string
		sddl string
		want bool
	}{
		{name: "owner only", sddl: "O:" + uid + "D:P(A;;FR;;;" + uid + ")", want: true},
		{name: "wrong owner", sddl: "O:BAD:P(A;;FR;;;" + uid + ")"},
		{name: "unprotected DACL", sddl: "O:" + uid + "D:(A;;FR;;;" + uid + ")"},
		{name: "null DACL", sddl: "O:" + uid + "D:NO_ACCESS_CONTROL"},
		{name: "empty DACL", sddl: "O:" + uid + "D:P"},
		{name: "other allow", sddl: "O:" + uid + "D:P(A;;FR;;;" + uid + ")(A;;FR;;;WD)"},
		{name: "plain deny plus owner allow", sddl: "O:" + uid + "D:P(D;;FR;;;WD)(A;;FR;;;" + uid + ")", want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			descriptor := windowsDescriptor(t, test.sddl)
			if got := protectedWindowsCredentialDescriptor(descriptor, user); got != test.want {
				t.Fatalf("protectedWindowsCredentialDescriptor=%v; want %v", got, test.want)
			}
		})
	}
	t.Run("unknown ACE", func(t *testing.T) {
		descriptor := windowsDescriptor(t, "O:"+uid+"D:P(A;;FR;;;"+uid+")")
		dacl, _, err := descriptor.DACL()
		if err != nil {
			t.Fatal(err)
		}
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, 0, &ace); err != nil {
			t.Fatal(err)
		}
		ace.Header.AceType = 0xff
		if protectedWindowsCredentialDescriptor(descriptor, user) {
			t.Fatal("unknown ACE type was accepted")
		}
	})
}

func TestProtectedWindowsCredentialObject(t *testing.T) {
	for _, test := range []struct {
		name       string
		fileType   uint32
		attributes uint32
		want       bool
	}{
		{name: "regular disk file", fileType: windows.FILE_TYPE_DISK, attributes: windows.FILE_ATTRIBUTE_NORMAL, want: true},
		{name: "directory", fileType: windows.FILE_TYPE_DISK, attributes: windows.FILE_ATTRIBUTE_DIRECTORY},
		{name: "reparse point", fileType: windows.FILE_TYPE_DISK, attributes: windows.FILE_ATTRIBUTE_REPARSE_POINT},
		{name: "reparse directory", fileType: windows.FILE_TYPE_DISK, attributes: windows.FILE_ATTRIBUTE_DIRECTORY | windows.FILE_ATTRIBUTE_REPARSE_POINT},
		{name: "character device", fileType: windows.FILE_TYPE_CHAR},
		{name: "pipe", fileType: windows.FILE_TYPE_PIPE},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := protectedWindowsCredentialObject(test.fileType, test.attributes); got != test.want {
				t.Fatalf("protectedWindowsCredentialObject=%v; want %v", got, test.want)
			}
		})
	}
}

func currentWindowsUserSID(t *testing.T) *windows.SID {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sid, err := user.User.Sid.Copy()
	if err != nil {
		t.Fatal(err)
	}
	return sid
}

func windowsDescriptor(t *testing.T, sddl string) *windows.SECURITY_DESCRIPTOR {
	t.Helper()
	descriptor, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatalf("SecurityDescriptorFromString(%q): %v", sddl, err)
	}
	return descriptor
}
