//go:build darwin

package secret

import (
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
