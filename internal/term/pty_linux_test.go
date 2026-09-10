//go:build linux

package term

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestReadPasswordContextCancellationRestoresPTYTermios(t *testing.T) {
	master, slave := openLinuxPTY(t)
	tty := newTTY(slave, slave)
	t.Cleanup(func() {
		// Closing the master first wakes the TTY input pump's blocking read.
		_ = master.Close()
		tty.Close()
	})

	before := getPTYTermios(t, slave)
	if before.Lflag&(unix.ECHO|unix.ICANON) != unix.ECHO|unix.ICANON {
		t.Fatalf("fresh PTY is not echoing canonical input: lflag = %#x", before.Lflag)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := ReadPasswordContext(ctx, tty, "token: ")
		done <- err
	}()

	during := waitForPTYTermios(t, slave, func(state *unix.Termios) bool {
		return state.Lflag&(unix.ECHO|unix.ICANON) == 0
	})
	if during.Lflag&(unix.ECHO|unix.ICANON) != 0 {
		t.Fatalf("password mode did not disable echo and canonical input: lflag = %#x", during.Lflag)
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("password read did not return after cancellation")
	}

	after := getPTYTermios(t, slave)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("termios was not restored after password cancellation:\n before: %+v\n  after: %+v", before, after)
	}
}

func TestViewerCancellationPhysicallyRestoresPTY(t *testing.T) {
	master, slave := openLinuxPTY(t)
	tty := newTTY(slave, slave)
	t.Cleanup(func() {
		_ = master.Close()
		tty.Close()
	})

	before := getPTYTermios(t, slave)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- ShowViewerContext(ctx, tty, []byte("claimed secret"), ViewerOpts{NoColor: true})
	}()

	waitForTTYModes(t, tty, true, true)
	during := getPTYTermios(t, slave)
	if during.Lflag&(unix.ECHO|unix.ICANON) != 0 {
		t.Fatalf("viewer did not enter raw terminal mode: lflag = %#x", during.Lflag)
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("viewer did not return after cancellation")
	}

	after := getPTYTermios(t, slave)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("termios was not restored after viewer cancellation:\n before: %+v\n  after: %+v", before, after)
	}

	waitForTTYModes(t, tty, false, false)
	wire := readAvailablePTY(t, master)
	for _, sequence := range [][]byte{
		[]byte("\x1b[?2004h"),
		[]byte("\x1b[?1049h"),
		[]byte("\x1b[?1049l"),
		[]byte("\x1b[?2004l"),
	} {
		if count := bytes.Count(wire, sequence); count != 1 {
			t.Fatalf("terminal sequence %q occurred %d times, want once; wire = %q", sequence, count, wire)
		}
	}
	if enter, leave := bytes.Index(wire, []byte("\x1b[?1049h")), bytes.Index(wire, []byte("\x1b[?1049l")); enter < 0 || leave < enter {
		t.Fatalf("alternate-screen cleanup preceded entry; wire = %q", wire)
	}
}

func openLinuxPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	masterFD, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Skipf("open /dev/ptmx: %v", err)
	}
	master := os.NewFile(uintptr(masterFD), "/dev/ptmx")
	if err := unix.IoctlSetPointerInt(masterFD, unix.TIOCSPTLCK, 0); err != nil {
		_ = master.Close()
		t.Fatalf("unlock PTY: %v", err)
	}
	ptyNumber, err := unix.IoctlGetInt(masterFD, unix.TIOCGPTN)
	if err != nil {
		_ = master.Close()
		t.Fatalf("get PTY number: %v", err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", ptyNumber), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		t.Fatalf("open PTY slave: %v", err)
	}
	return master, slave
}

func getPTYTermios(t *testing.T, f *os.File) *unix.Termios {
	t.Helper()
	state, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatalf("get termios: %v", err)
	}
	return state
}

func waitForPTYTermios(t *testing.T, f *os.File, ready func(*unix.Termios) bool) *unix.Termios {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		state := getPTYTermios(t, f)
		if ready(state) {
			return state
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for PTY termios transition")
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForTTYModes(t *testing.T, tty *TTY, paste, alternate bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		tty.mu.Lock()
		gotPaste, gotAlternate := tty.bracketedPaste, tty.alternateScreen
		tty.mu.Unlock()
		if gotPaste == paste && gotAlternate == alternate {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for terminal modes paste=%v alternate=%v; got paste=%v alternate=%v",
				paste, alternate, gotPaste, gotAlternate)
		}
		time.Sleep(time.Millisecond)
	}
}

func readAvailablePTY(t *testing.T, master *os.File) []byte {
	t.Helper()
	fd := int(master.Fd())
	if err := unix.SetNonblock(fd, true); err != nil {
		t.Fatalf("make PTY master nonblocking: %v", err)
	}
	defer func() {
		if err := unix.SetNonblock(fd, false); err != nil {
			t.Errorf("restore PTY master blocking mode: %v", err)
		}
	}()

	var wire []byte
	buf := make([]byte, 4096)
	for {
		n, err := unix.Read(fd, buf)
		if n > 0 {
			wire = append(wire, buf[:n]...)
		}
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
			return wire
		}
		if err != nil {
			t.Fatalf("read PTY master: %v", err)
		}
		if n == 0 {
			return wire
		}
	}
}
