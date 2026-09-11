//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/burnerpad/burnerpad-cli/internal/secret"
	"golang.org/x/sys/windows"
)

func insecureCredentialFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	file, err := secret.CreateExclusive(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(content)); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	uid := user.User.Sid.String()
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + uid + ")(A;;GR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
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
	return path
}

func pipeDescriptor(t *testing.T, content string) int {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	var duplicate windows.Handle
	process := windows.CurrentProcess()
	if err := windows.DuplicateHandle(process, windows.Handle(reader.Fd()), process, &duplicate,
		0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		_ = windows.CloseHandle(duplicate)
		_ = writer.Close()
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(content)); err != nil {
		_ = windows.CloseHandle(duplicate)
		_ = writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		_ = windows.CloseHandle(duplicate)
		t.Fatal(err)
	}
	return int(duplicate)
}
