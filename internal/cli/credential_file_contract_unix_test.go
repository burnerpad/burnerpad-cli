//go:build linux || darwin

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func insecureCredentialFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func pipeDescriptor(t *testing.T, content string) int {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := unix.Dup(int(reader.Fd()))
	if closeErr := reader.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(content)); err != nil {
		_ = unix.Close(descriptor)
		_ = writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		_ = unix.Close(descriptor)
		t.Fatal(err)
	}
	return descriptor
}
