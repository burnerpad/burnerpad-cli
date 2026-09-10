package term

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestReadLineContextUsesSharedPumpAndCoalescesCRLF(t *testing.T) {
	tty, inW, _ := pipeTTY(t)
	go func() {
		_, _ = io.WriteString(inW, "first\r\nsecond\n")
		_ = inW.Close()
	}()
	first, err := ReadLineContext(context.Background(), tty, "first: ")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ReadLineContext(context.Background(), tty, "second: ")
	if err != nil {
		t.Fatal(err)
	}
	if first != "first" || second != "second" {
		t.Fatalf("lines = %q, %q", first, second)
	}
}

func TestReadLineContextCancellation(t *testing.T) {
	tty, _, outR := pipeTTY(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadLineContext(ctx, tty, "value: "); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if err := tty.out.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := io.ReadAll(outR); err != nil || len(got) != 0 {
		t.Fatalf("pre-canceled prompt output = %q, err = %v", got, err)
	}
}

func TestReadPasswordContextPreCanceledAndSetupFailure(t *testing.T) {
	t.Run("pre-canceled", func(t *testing.T) {
		tty, _, outR := pipeTTY(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := ReadPasswordContext(ctx, tty, "token: "); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		if err := tty.out.Close(); err != nil {
			t.Fatal(err)
		}
		if got, err := io.ReadAll(outR); err != nil || len(got) != 0 {
			t.Fatalf("pre-canceled prompt output = %q, err = %v", got, err)
		}
	})
	t.Run("mode setup", func(t *testing.T) {
		tty, _, _ := pipeTTY(t)
		if _, err := ReadPasswordContext(context.Background(), tty, "token: "); err == nil || errors.Is(err, ErrInterrupted) {
			t.Fatalf("mode setup err = %v, want underlying terminal error", err)
		}
	})
}

func TestReadLineContextRawEditingAndPasteWipe(t *testing.T) {
	tty, inW, _ := pipeTTY(t)
	payload := []byte("secret")
	tty.startPump()
	tty.events <- rn('a')
	tty.events <- rn('b')
	tty.events <- kd(KindBackspace)
	tty.events <- Event{Kind: KindPaste, Paste: payload}
	tty.events <- kd(KindEnter)
	line, err := tty.readLineContext(context.Background(), 64)
	_ = inW.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(line) != "asecret" {
		t.Fatalf("line = %q", line)
	}
	for i, b := range payload {
		if b != 0 {
			t.Fatalf("paste byte %d not wiped", i)
		}
	}
}

func TestReadLineContextBoundsWipesAndDrainsTheWholeLine(t *testing.T) {
	tty := &TTY{events: make(chan Event, 16), winch: make(chan struct{}, 1)}
	tty.pumpOnce.Do(func() {}) // events are injected directly; do not start an fd pump
	payload := []byte("secret")
	for _, ev := range []Event{
		rn('a'),
		{Kind: KindPaste, Paste: payload},
		rn('x'),
		kd(KindEnter),
		rn('o'), rn('k'), kd(KindEnter),
	} {
		tty.events <- ev
	}
	if line, err := tty.readLineContext(context.Background(), 3); !errors.Is(err, ErrInputTooLong) || line != nil {
		t.Fatalf("overflow line = %q, err=%v", line, err)
	}
	for i, b := range payload {
		if b != 0 {
			t.Fatalf("overflow paste byte %d was not wiped", i)
		}
	}
	line, err := tty.readLineContext(context.Background(), 3)
	if err != nil || string(line) != "ok" {
		t.Fatalf("line after drained overflow = %q, err=%v", line, err)
	}
}

func TestReadLineContextAcceptsExactLimitAndCancelsWhileDraining(t *testing.T) {
	t.Run("exact limit", func(t *testing.T) {
		tty := &TTY{events: make(chan Event, 2), winch: make(chan struct{}, 1)}
		tty.pumpOnce.Do(func() {})
		payload := []byte("abc")
		tty.events <- Event{Kind: KindPaste, Paste: payload}
		tty.events <- kd(KindEnter)
		line, err := tty.readLineContext(context.Background(), 3)
		if err != nil || string(line) != "abc" {
			t.Fatalf("exact-limit line = %q, err=%v", line, err)
		}
		for i, b := range payload {
			if b != 0 {
				t.Fatalf("exact-limit paste byte %d was not wiped", i)
			}
		}
	})

	t.Run("cancellation while draining", func(t *testing.T) {
		tty := &TTY{events: make(chan Event), winch: make(chan struct{}, 1)}
		tty.pumpOnce.Do(func() {})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := tty.readLineContext(ctx, 3)
			done <- err
		}()
		tty.events <- kd(KindInputTooLong)
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v, want context.Canceled", err)
		}
	})
}
