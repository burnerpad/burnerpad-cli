//go:build linux || darwin

package secret

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestOpenCredentialFileAcceptsOwnerOnlyRegularFiles(t *testing.T) {
	for _, mode := range []os.FileMode{0o600, 0o400} {
		t.Run(mode.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credential")
			if err := os.WriteFile(path, []byte("credential"), mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			file, err := OpenCredentialFile(path)
			if err != nil {
				t.Fatalf("OpenCredentialFile: %v", err)
			}
			got, readErr := io.ReadAll(file)
			closeErr := file.Close()
			if readErr != nil || closeErr != nil {
				t.Fatalf("read=%v close=%v", readErr, closeErr)
			}
			if string(got) != "credential" {
				t.Fatalf("content=%q", got)
			}
		})
	}
}

func TestOpenCredentialFileRejectsUnsafeUnixObjects(t *testing.T) {
	for _, mode := range []os.FileMode{0o640, 0o604, 0o666} {
		t.Run("mode "+mode.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credential")
			if err := os.WriteFile(path, []byte("credential"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			assertUnprotectedCredentialFile(t, path)
		})
	}

	t.Run("directory", func(t *testing.T) {
		assertUnprotectedCredentialFile(t, t.TempDir())
	})
	t.Run("device", func(t *testing.T) {
		assertUnprotectedCredentialFile(t, "/dev/null")
	})
	t.Run("symlink", func(t *testing.T) {
		directory := t.TempDir()
		target := filepath.Join(directory, "target")
		link := filepath.Join(directory, "link")
		if err := os.WriteFile(target, []byte("credential"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		assertUnprotectedCredentialFile(t, link)
	})
	t.Run("wrong owner", func(t *testing.T) {
		if os.Geteuid() != 0 {
			t.Skip("changing file ownership requires root")
		}
		path := filepath.Join(t.TempDir(), "credential")
		if err := os.WriteFile(path, []byte("credential"), 0o600); err != nil {
			t.Fatal(err)
		}
		owner := 1
		if owner == os.Geteuid() {
			owner++
		}
		if err := os.Chown(path, owner, -1); err != nil {
			t.Fatal(err)
		}
		assertUnprotectedCredentialFile(t, path)
	})
	if _, err := OpenCredentialFile(filepath.Join(t.TempDir(), "missing")); err == nil || errors.Is(err, ErrUnprotectedCredentialFile) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing-file error=%v", err)
	}
}

func TestOpenCredentialFileRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		file, err := OpenCredentialFile(path)
		if file != nil {
			_ = file.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrUnprotectedCredentialFile) {
			t.Fatalf("FIFO error=%v; want ErrUnprotectedCredentialFile", err)
		}
	case <-time.After(time.Second):
		t.Fatal("opening a FIFO blocked")
	}
}

func TestProtectedUnixCredentialStat(t *testing.T) {
	const user = uint32(1234)
	for _, test := range []struct {
		name  string
		mode  uint32
		owner uint32
		want  bool
	}{
		{name: "0600 regular", mode: uint32(unix.S_IFREG) | 0o600, owner: user, want: true},
		{name: "0400 regular", mode: uint32(unix.S_IFREG) | 0o400, owner: user, want: true},
		{name: "group readable", mode: uint32(unix.S_IFREG) | 0o640, owner: user},
		{name: "other readable", mode: uint32(unix.S_IFREG) | 0o604, owner: user},
		{name: "wrong owner", mode: uint32(unix.S_IFREG) | 0o600, owner: user + 1},
		{name: "directory", mode: uint32(unix.S_IFDIR) | 0o600, owner: user},
		{name: "fifo", mode: uint32(unix.S_IFIFO) | 0o600, owner: user},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := protectedUnixCredentialStat(test.mode, test.owner, user); got != test.want {
				t.Fatalf("protectedUnixCredentialStat=%v; want %v", got, test.want)
			}
		})
	}
}
