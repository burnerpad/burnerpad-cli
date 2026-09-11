//go:build darwin

package secret

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOpenCredentialFileRejectsDarwinExtendedACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(path, []byte("credential"), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("/bin/chmod", "+a", "everyone allow read", path).CombinedOutput(); err != nil {
		t.Fatalf("add test ACL: %v: %s", err, output)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("ACL unexpectedly changed BSD mode to %o; test requires independent ACL access", info.Mode().Perm())
	}
	assertUnprotectedCredentialFile(t, path)
}

func TestDarwinFileSecurityHasACL(t *testing.T) {
	fileSecurity := func(entryCount uint32) []byte {
		size := darwinFileSecurityHeaderSize
		if entryCount != darwinFileSecurityNoACL {
			size += int(entryCount) * darwinFileSecurityACEBytes
		}
		value := make([]byte, size)
		binary.LittleEndian.PutUint32(value[:4], darwinFileSecurityMagic)
		binary.LittleEndian.PutUint32(value[darwinFileSecurityCountOffset:], entryCount)
		return value
	}
	for _, test := range []struct {
		name    string
		value   []byte
		wantACL bool
		wantErr bool
	}{
		{name: "no record"},
		{name: "security record without ACL", value: fileSecurity(darwinFileSecurityNoACL)},
		{name: "empty ACL", value: fileSecurity(0), wantACL: true},
		{name: "one ACE", value: fileSecurity(1), wantACL: true},
		{name: "short record", value: make([]byte, darwinFileSecurityHeaderSize-1), wantErr: true},
		{name: "wrong magic", value: make([]byte, darwinFileSecurityHeaderSize), wantErr: true},
		{name: "size mismatch", value: fileSecurity(1)[:darwinFileSecurityHeaderSize], wantErr: true},
		{name: "too many ACEs", value: fileSecurity(darwinFileSecurityMaxEntries + 1), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := darwinFileSecurityHasACL(test.value)
			if got != test.wantACL || (err != nil) != test.wantErr {
				t.Fatalf("darwinFileSecurityHasACL = (%v, %v), want (%v, error=%v)", got, err, test.wantACL, test.wantErr)
			}
		})
	}
}
