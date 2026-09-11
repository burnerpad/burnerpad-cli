//go:build linux || darwin

// White-box, same package, matching the repo's test style. Tagged for the
// platforms where Harden actually changes process state; the Windows
// CreateExclusive DACL path is compile-checked by the cross-build gate and
// exercised by the manual Windows smoke checklist.
package secret

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func TestHarden(t *testing.T) {
	Harden()

	var r syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_CORE, &r); err != nil {
		t.Fatalf("Getrlimit(RLIMIT_CORE): %v", err)
	}
	if r.Cur != 0 || r.Max != 0 {
		t.Errorf("RLIMIT_CORE after Harden = {Cur:%d Max:%d}, want {0 0}", r.Cur, r.Max)
	}

	if runtime.GOOS == "linux" {
		status, err := os.ReadFile("/proc/self/status")
		if err != nil {
			t.Fatalf("read /proc/self/status: %v", err)
		}
		found := false
		for _, line := range strings.Split(string(status), "\n") {
			if v, ok := strings.CutPrefix(line, "CoreDumping:"); ok {
				found = true
				if strings.TrimSpace(v) != "0" {
					t.Errorf("CoreDumping = %q, want 0 (PR_SET_DUMPABLE not applied)", strings.TrimSpace(v))
				}
			}
		}
		if !found {
			t.Skip("kernel too old for CoreDumping field (< 4.15)")
		}
	}
}

// Harden must stay safe to call twice (a second call is a bug elsewhere, but
// it must never be the crash).
func TestHardenIdempotent(t *testing.T) {
	Harden()
	Harden()
}

func TestCreateExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.bin")

	f, err := CreateExclusive(path)
	if err != nil {
		t.Fatalf("CreateExclusive(fresh path): %v", err)
	}
	if _, err := f.Write([]byte("payload")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 0600", perm)
	}

	f2, err := CreateExclusive(path)
	if err == nil {
		f2.Close()
		t.Fatal("second CreateExclusive on the same path succeeded, want error")
	}
	if !errors.Is(err, os.ErrExist) {
		t.Errorf("second CreateExclusive error = %v, want errors.Is(err, os.ErrExist)", err)
	}
}

func TestWipe(t *testing.T) {
	b := []byte{1, 2, 3, 4, 5}
	Wipe(b)
	if !bytes.Equal(b, make([]byte, 5)) {
		t.Errorf("Wipe left %v, want all zero", b)
	}
	Wipe(nil)      // must not panic
	Wipe([]byte{}) // must not panic
	Wipe(b[:0])    // must not panic
	Wipe(b)        // re-wipe of zeroed slice is fine
}

// New takes ownership: no defensive copy, so the Buffer aliases the caller's
// slice and Wipe reaches the caller's bytes.
func TestNewOwnership(t *testing.T) {
	orig := []byte("ownership-probe")
	s := New(orig)

	if got := s.Bytes(); len(got) != len(orig) || &got[0] != &orig[0] {
		t.Fatal("New copied the slice; it must take ownership of the caller's backing array")
	}
	if s.Len() != len(orig) {
		t.Errorf("Len = %d, want %d", s.Len(), len(orig))
	}

	s.Wipe()
	if !bytes.Equal(orig, make([]byte, len(orig))) {
		t.Error("Wipe did not zero the caller's slice — ownership semantics broken")
	}
	if s.Bytes() != nil {
		t.Error("Bytes after Wipe should be nil")
	}
	if s.Len() != 0 {
		t.Error("Len after Wipe should be 0")
	}
}

func TestDoubleWipe(t *testing.T) {
	s := New([]byte("twice"))
	s.Wipe()
	s.Wipe() // must not panic
	if s.Bytes() != nil || s.Len() != 0 {
		t.Error("Buffer not empty after double Wipe")
	}
}

func TestNilSafe(t *testing.T) {
	var s *Buffer
	if s.Bytes() != nil {
		t.Error("nil.Bytes() != nil")
	}
	if s.Len() != 0 {
		t.Error("nil.Len() != 0")
	}
	s.Wipe() // must not panic
	if got := fmt.Sprintf("%v", s); got != "[redacted]" {
		t.Errorf("%%v of nil *Buffer = %q, want [redacted]", got)
	}
}

func TestRedaction(t *testing.T) {
	const probe = "s3kr1t-probe"
	s := New([]byte(probe))
	defer s.Wipe()

	for _, verb := range []string{"%v", "%s", "%#v", "%+v"} {
		got := fmt.Sprintf(verb, s)
		if got != "[redacted]" {
			t.Errorf("Sprintf(%q) = %q, want [redacted]", verb, got)
		}
	}

	// Exercise the accidental-wrap path: fmt.Errorf must leak nothing,
	// including through further %w wrapping.
	err := fmt.Errorf("outer: %w", fmt.Errorf("open failed: %v", s))
	if msg := err.Error(); strings.Contains(msg, probe) {
		t.Error("secret bytes leaked through fmt.Errorf wrapping")
	} else if !strings.Contains(msg, "[redacted]") {
		t.Errorf("wrapped error = %q, want it to carry [redacted]", msg)
	}
}
