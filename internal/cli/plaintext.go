package cli

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/burnerpad/burnerpad-cli/envelope"
	"github.com/burnerpad/burnerpad-cli/internal/secret"
	"github.com/burnerpad/burnerpad-cli/internal/term"
)

type plaintextDelivery struct {
	out                    *reservedFile
	server                 string
	claimedWithoutRecovery bool
	retry                  passphraseRetry
}

type passphraseRetry interface {
	canRetryPassphrase() bool
	readRetryPassphrase() ([]byte, error)
}

// decryptAndDeliver owns the shared authenticated-plaintext pipeline after a
// caller has acquired a blob and its initial phrase. The caller still owns and
// wipes that initial phrase across any earlier failure; this method wipes each
// phrase it uses, including a replacement read during local retry.
func (a *application) decryptAndDeliver(blob, phrase []byte, delivery plaintextDelivery) error {
	defer func() { secret.Wipe(phrase) }()
	claimed := delivery.server != ""
	plaintext, err := envelope.DecryptPassphrase(blob, phrase)
	for err == envelope.ErrAuthFail && delivery.retry.canRetryPassphrase() {
		secret.Wipe(phrase)
		if claimed {
			fmt.Fprintln(a.env.Stderr, "burnerpad: the phrase did not open it; try again locally")
		}
		phrase, err = delivery.retry.readRetryPassphrase()
		if err != nil {
			break
		}
		plaintext, err = envelope.DecryptPassphrase(blob, phrase)
	}
	if err != nil {
		if delivery.claimedWithoutRecovery {
			a.warn("the claimed ciphertext is being discarded; use --keep-blob before claiming when recovery may be needed")
		}
		return mapDecryptError(err, delivery.server)
	}
	defer secret.Wipe(plaintext)
	if !utf8.Valid(plaintext) {
		if delivery.claimedWithoutRecovery {
			a.warn("the claimed ciphertext is being discarded")
		}
		return commandError{
			exit: 5, code: "plaintext_invalid",
			message: "the authenticated plaintext is not valid UTF-8 text",
			server:  delivery.server,
		}
	}
	if err := a.deliverPlaintext(delivery.out, delivery.server, plaintext); err != nil {
		if delivery.claimedWithoutRecovery {
			a.warn("the claimed ciphertext is being discarded")
		}
		return attachServer(err, delivery.server)
	}
	return nil
}

func (a *application) readRetryPassphrase() ([]byte, error) {
	return a.readPassphrase(true, "", -1)
}

func (a *application) prepareDestination(path string) (*reservedFile, error) {
	var out *reservedFile
	if path != "" {
		var err error
		out, err = reserve(path)
		if err != nil {
			return nil, err
		}
	}
	if !a.cfg.json && out == nil && a.env.StdoutTTY {
		if _, err := a.terminal(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (a *application) deliverPlaintext(out *reservedFile, server string, plaintext []byte) error {
	// Once plaintext is ready—especially after a one-time claim—a signal that
	// arrived during local decryption must not erase the destination as soon as
	// it is entered. Complete that already-pending handoff; the viewer uses
	// persistent plain output instead of waiting on the alternate screen. When
	// delivery begins before cancellation, the Run context still cancels its
	// interactive wait.
	deliveryCtx := a.context()
	pendingCancellation := deliveryCtx.Err() != nil
	if pendingCancellation {
		deliveryCtx = context.WithoutCancel(deliveryCtx)
	}
	switch {
	case a.cfg.json:
		if server != "" {
			if err := writeJSON(a.env.Stdout, revealResult{Status: "revealed", Server: server, Plaintext: string(plaintext)}); err != nil {
				return local("cannot write JSON output")
			}
		} else {
			if err := writeJSON(a.env.Stdout, decryptResult{Status: "decrypted", Plaintext: string(plaintext)}); err != nil {
				return local("cannot write JSON output")
			}
		}
	case out != nil:
		if err := out.write(plaintext); err != nil {
			return err
		}
		out.written = true
	case a.env.StdoutTTY:
		t, err := a.terminal()
		if err != nil {
			return err
		}
		if err := term.ShowViewerContext(deliveryCtx, t, plaintext, term.ViewerOpts{
			NoAlt: pendingCancellation || a.cfg.plain, NoColor: a.cfg.noColor,
		}); err != nil {
			if errors.Is(err, term.ErrInterrupted) || errors.Is(err, context.Canceled) {
				return interrupted(err)
			}
			return localCause("cannot display plaintext", err)
		}
	default:
		if _, err := a.env.Stdout.Write(plaintext); err != nil {
			return local("cannot write plaintext")
		}
	}
	return nil
}
