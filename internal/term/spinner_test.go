package term

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// The §11.6 gate: an operation that finishes before the delay draws NOTHING
// — fast machines never see a spinner.
func TestSpinnerGateSuppressesFastOps(t *testing.T) {
	oldDelay := spinnerDelay
	spinnerDelay = 5 * time.Second // scheduling-jitter-proof stand-in for 100 ms
	defer func() { spinnerDelay = oldDelay }()

	var buf bytes.Buffer
	stop := Spinner(&buf, "deriving key (PBKDF2 · 600,000 rounds)…")
	time.Sleep(10 * time.Millisecond) // "fast operation": ends before the gate
	stop()
	if buf.Len() != 0 {
		t.Fatalf("spinner drew before the gate: %q", buf.String())
	}
}

// Past the gate it renders the message and stop erases the line.
func TestSpinnerRendersAfterGate(t *testing.T) {
	oldDelay, oldTick := spinnerDelay, spinnerInterval
	spinnerDelay, spinnerInterval = 10*time.Millisecond, 20*time.Millisecond
	defer func() { spinnerDelay, spinnerInterval = oldDelay, oldTick }()

	var buf bytes.Buffer
	stop := Spinner(&buf, "sealing…")
	time.Sleep(120 * time.Millisecond)
	stop()
	out := buf.String()
	if !strings.Contains(out, "sealing…") {
		t.Fatalf("message never rendered: %q", out)
	}
	if !strings.HasSuffix(out, "\r\x1b[K") {
		t.Fatalf("stop did not erase the line: %q", out)
	}
	if !strings.ContainsAny(out, spinnerFrames) {
		t.Fatalf("no spinner frame rendered: %q", out)
	}
}

// stop is idempotent and safe to call twice.
func TestSpinnerStopIdempotent(t *testing.T) {
	var buf bytes.Buffer
	stop := Spinner(&buf, "x")
	stop()
	stop()
}
