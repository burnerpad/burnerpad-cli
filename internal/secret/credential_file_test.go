package secret

import (
	"errors"
	"testing"
)

func assertUnprotectedCredentialFile(t *testing.T, path string) {
	t.Helper()
	file, err := OpenCredentialFile(path)
	if file != nil {
		_ = file.Close()
		t.Fatal("unsafe credential file was opened")
	}
	if !errors.Is(err, ErrUnprotectedCredentialFile) {
		t.Fatalf("error=%v; want ErrUnprotectedCredentialFile", err)
	}
}
