package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/burnerpad/burnerpad-cli/envelope"
	"github.com/burnerpad/burnerpad-cli/internal/term"
)

func TestDecryptAndDeliverModes(t *testing.T) {
	blob := envelope.EncryptPassphrase([]byte(contractPhrase), []byte("shared plaintext"))
	for _, test := range []struct {
		name, server, want string
	}{
		{name: "offline", want: "{\"status\":\"decrypted\",\"plaintext\":\"shared plaintext\"}\n"},
		{name: "claimed", server: "https://example.com", want: "{\"status\":\"revealed\",\"server\":\"https://example.com\",\"plaintext\":\"shared plaintext\"}\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			phrase := []byte(contractPhrase)
			a := &application{env: Env{Stdout: &stdout, Stderr: &stderr}, cfg: config{json: true}}
			if err := a.decryptAndDeliver(blob, phrase, plaintextDelivery{server: test.server, retry: &stubPassphraseRetry{}}); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != test.want || stderr.Len() != 0 {
				t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if !bytes.Equal(phrase, make([]byte, len(phrase))) {
				t.Fatal("initial phrase was not wiped")
			}
		})
	}
}

func TestDecryptAndDeliverRetriesLocally(t *testing.T) {
	const notice = "burnerpad: the phrase did not open it; try again locally\n"
	blob := envelope.EncryptPassphrase([]byte(contractPhrase), []byte("opened after retry"))
	for _, test := range []struct {
		name, server, stderr string
	}{
		{name: "offline"},
		{name: "claimed", server: "https://example.com", stderr: notice},
	} {
		t.Run(test.name, func(t *testing.T) {
			initial := []byte("freeway faucet unnoticed energy emoticon elves enormous")
			replacement := []byte(contractPhrase)
			retry := &stubPassphraseRetry{available: true, phrase: replacement}
			var stdout, stderr bytes.Buffer
			a := &application{env: Env{Stdout: &stdout, Stderr: &stderr}, cfg: config{json: true}}
			if err := a.decryptAndDeliver(blob, initial, plaintextDelivery{server: test.server, retry: retry}); err != nil {
				t.Fatal(err)
			}
			if retry.reads != 1 || stderr.String() != test.stderr || !bytes.Contains(stdout.Bytes(), []byte("opened after retry")) {
				t.Fatalf("reads=%d stdout=%q stderr=%q", retry.reads, stdout.String(), stderr.String())
			}
			for name, phrase := range map[string][]byte{"initial": initial, "replacement": replacement} {
				if !bytes.Equal(phrase, make([]byte, len(phrase))) {
					t.Fatalf("%s phrase was not wiped", name)
				}
			}
		})
	}
}

func TestDecryptAndDeliverPreservesRetryReadError(t *testing.T) {
	blob := envelope.EncryptPassphrase([]byte(contractPhrase), []byte("secret"))
	initial := []byte("freeway faucet unnoticed energy emoticon elves enormous")
	retryErr := localCause("cannot read retry phrase", io.ErrClosedPipe)
	retry := &stubPassphraseRetry{available: true, err: retryErr}
	var stdout, stderr bytes.Buffer
	a := &application{env: Env{Stdout: &stdout, Stderr: &stderr}, cfg: config{json: true}}
	err := a.decryptAndDeliver(blob, initial, plaintextDelivery{server: "https://example.com", retry: retry})
	var command commandError
	if !errors.As(err, &command) || command.exit != 3 || command.code != "local_io_failed" ||
		command.server != "https://example.com" || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("error=%#v, want server-attached retry I/O error", err)
	}
	if retry.reads != 1 || stdout.Len() != 0 || stderr.String() != "burnerpad: the phrase did not open it; try again locally\n" {
		t.Fatalf("reads=%d stdout=%q stderr=%q", retry.reads, stdout.String(), stderr.String())
	}
	if !bytes.Equal(initial, make([]byte, len(initial))) {
		t.Fatal("rejected phrase was not wiped before retry")
	}
}

func TestApplicationRetryPassphrasePreservesInterruption(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a := &application{ctx: ctx, tty: new(term.TTY)}
	if !a.canRetryPassphrase() {
		t.Fatal("cached terminal was not available for retry")
	}
	_, err := a.readRetryPassphrase()
	var command commandError
	if !errors.As(err, &command) || command.exit != 130 || !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%#v, want interruption preserving context cancellation", err)
	}
}

func TestDecryptAndDeliverFailureContracts(t *testing.T) {
	const (
		server       = "https://example.com"
		longWarning  = "burnerpad: warning: the claimed ciphertext is being discarded; use --keep-blob before claiming when recovery may be needed\n"
		shortWarning = "burnerpad: warning: the claimed ciphertext is being discarded\n"
	)
	for _, test := range []struct {
		name, server, phrase, stderr string
		plaintext                    []byte
		unrecoverable, failOutput    bool
		exit                         int
		code                         string
	}{
		{name: "claimed authentication", server: server, phrase: "freeway faucet unnoticed energy emoticon elves enormous", plaintext: []byte("secret"), unrecoverable: true, exit: 5, code: "passphrase_failed", stderr: longWarning},
		{name: "preserved claim authentication", server: server, phrase: "freeway faucet unnoticed energy emoticon elves enormous", plaintext: []byte("secret"), exit: 5, code: "passphrase_failed"},
		{name: "offline authentication", phrase: "freeway faucet unnoticed energy emoticon elves enormous", plaintext: []byte("secret"), exit: 5, code: "passphrase_failed"},
		{name: "claimed UTF-8", server: server, phrase: contractPhrase, plaintext: []byte{0xff}, unrecoverable: true, exit: 5, code: "plaintext_invalid", stderr: shortWarning},
		{name: "offline UTF-8", phrase: contractPhrase, plaintext: []byte{0xff}, exit: 5, code: "plaintext_invalid"},
		{name: "claimed delivery", server: server, phrase: contractPhrase, plaintext: []byte("secret"), unrecoverable: true, failOutput: true, exit: 3, code: "local_io_failed", stderr: shortWarning},
		{name: "offline delivery", phrase: contractPhrase, plaintext: []byte("secret"), failOutput: true, exit: 3, code: "local_io_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			blob := envelope.EncryptPassphrase([]byte(contractPhrase), test.plaintext)
			phrase := []byte(test.phrase)
			var stdout, stderr bytes.Buffer
			var output io.Writer = &stdout
			if test.failOutput {
				output = failingWriter{}
			}
			retry := &stubPassphraseRetry{}
			a := &application{env: Env{Stdout: output, Stderr: &stderr}, cfg: config{json: true}}
			err := a.decryptAndDeliver(blob, phrase, plaintextDelivery{
				server: test.server, claimedWithoutRecovery: test.unrecoverable, retry: retry,
			})
			var command commandError
			if !errors.As(err, &command) || command.exit != test.exit || command.code != test.code || command.server != test.server {
				t.Fatalf("error=%#v, want exit=%d code=%q server=%q", err, test.exit, test.code, test.server)
			}
			if stdout.Len() != 0 || stderr.String() != test.stderr {
				t.Fatalf("stdout=%q stderr=%q, want stderr=%q", stdout.String(), stderr.String(), test.stderr)
			}
			if !bytes.Equal(phrase, make([]byte, len(phrase))) {
				t.Fatal("initial phrase was not wiped")
			}
		})
	}
}

type stubPassphraseRetry struct {
	available bool
	phrase    []byte
	err       error
	reads     int
}

func (r *stubPassphraseRetry) canRetryPassphrase() bool { return r.available }

func (r *stubPassphraseRetry) readRetryPassphrase() ([]byte, error) {
	r.reads++
	r.available = false
	return r.phrase, r.err
}
