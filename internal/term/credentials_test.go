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
	line, err := tty.readLineContext(context.Background())
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
