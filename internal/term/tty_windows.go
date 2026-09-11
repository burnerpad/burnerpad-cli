//go:build windows

package term

import (
	"os"

	"golang.org/x/sys/windows"
)

// platformState remembers the console output mode enableVT changed so
// EmergencyRestore can put it back from the signal path.
type platformState struct {
	outHandle windows.Handle
	outMode   uint32
	saved     bool
}

// openTTY opens CONIN$/CONOUT$ directly so prompts survive stdio redirection
// exactly as /dev/tty does on Unix.
func openTTY() (*TTY, error) {
	in, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		in.Close()
		return nil, err
	}
	// t.winch never fires: Windows has no SIGWINCH; resize repaint is
	// a Unix-only nicety.
	return newTTY(in, out), nil
}

// enableVT turns on ANSI processing for the console. x/term's MakeRaw
// already sets ENABLE_VIRTUAL_TERMINAL_INPUT on the input handle; output
// needs ENABLE_VIRTUAL_TERMINAL_PROCESSING, which legacy conhost refuses —
// that error triggers the documented --plain fallback.
func (t *TTY) enableVT() (restore func(), err error) {
	h := windows.Handle(t.out.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return nil, err
	}
	want := mode | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING | windows.ENABLE_PROCESSED_OUTPUT
	if err := windows.SetConsoleMode(h, want); err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.plat = platformState{outHandle: h, outMode: mode, saved: true}
	t.mu.Unlock()
	return func() {
		_ = windows.SetConsoleMode(h, mode)
		t.mu.Lock()
		t.plat.saved = false
		t.mu.Unlock()
	}, nil
}

// makePasswordInput mirrors x/term's legacy-compatible password posture but
// leaves the state lifetime with TTY. In particular it does not request
// ENABLE_VIRTUAL_TERMINAL_INPUT, which older conhost versions can reject.
func (t *TTY) makePasswordInput() (restore func(), err error) {
	h := windows.Handle(t.in.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return nil, err
	}
	want := mode &^ (windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT)
	want |= windows.ENABLE_PROCESSED_INPUT
	if err := windows.SetConsoleMode(h, want); err != nil {
		return nil, err
	}
	return func() { _ = windows.SetConsoleMode(h, mode) }, nil
}

func (t *TTY) platformEmergency() {
	t.mu.Lock()
	p := t.plat
	t.plat.saved = false
	t.mu.Unlock()
	if p.saved {
		_ = windows.SetConsoleMode(p.outHandle, p.outMode)
	}
}
