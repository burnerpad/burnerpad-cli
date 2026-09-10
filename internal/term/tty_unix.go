//go:build !windows

package term

import (
	"os"
	"os/signal"
	"syscall"
)

// platformState: termios state is fully captured by x/term; nothing extra.
// The TTY.plat field exists for the Windows build, where this type carries
// the saved console output mode — hence unused here by design.
//
//lint:ignore U1000 counterpart of tty_windows.go's platformState — TTY.plat must exist on every GOOS for the struct to compile
type platformState struct{}

// openTTY opens the controlling terminal read-write. One file serves both
// directions; absence (no controlling terminal) is the error contract.
func openTTY() (*TTY, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	t := newTTY(f, f)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGWINCH)
	go func() {
		for range sig {
			select {
			case t.winch <- struct{}{}: // coalesced: one pending tick is enough
			default:
			}
		}
	}()
	t.stopWinch = func() {
		signal.Stop(sig)
		close(sig)
	}
	return t, nil
}

// enableVT is a no-op on Unix: any terminal that gets this far speaks VT.
func (t *TTY) enableVT() (restore func(), err error) {
	return func() {}, nil
}

func (t *TTY) platformEmergency() {}
