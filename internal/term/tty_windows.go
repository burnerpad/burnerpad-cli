//go:build windows

package term

import (
	"os"

	"golang.org/x/sys/windows"
)

// platformState remembers the console output mode enableVT changed, so
// EmergencyRestore can put it back from the signal path (A8).
type platformState struct {
	outHandle windows.Handle
	outMode   uint32
	saved     bool
}

// openTTY opens the console devices directly (§5.1: CONIN$/CONOUT$), so
// prompts survive stdio redirection exactly as /dev/tty does on Unix.
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
	// t.winch never fires: Windows has no SIGWINCH (§21); resize repaint is
	// a Unix-only nicety.
	return newTTY(in, out), nil
}

// enableVT turns on ANSI processing for the console (§21). x/term's MakeRaw
// already sets ENABLE_VIRTUAL_TERMINAL_INPUT on the input handle; output
// needs ENABLE_VIRTUAL_TERMINAL_PROCESSING, which legacy conhost refuses —
// that error is the documented --plain fallback trigger (§7.6).
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

func (t *TTY) platformEmergency() {
	t.mu.Lock()
	p := t.plat
	t.plat.saved = false
	t.mu.Unlock()
	if p.saved {
		_ = windows.SetConsoleMode(p.outHandle, p.outMode)
	}
}
