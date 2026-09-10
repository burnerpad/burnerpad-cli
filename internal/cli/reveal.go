package cli

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/burnerpad/burnerpad-cli/envelope"
	"github.com/burnerpad/burnerpad-cli/internal/api"
	"github.com/burnerpad/burnerpad-cli/internal/id"
	"github.com/burnerpad/burnerpad-cli/internal/secret"
	"github.com/burnerpad/burnerpad-cli/internal/term"
)

func runReveal(a *application, flags *revealFlags, positionals []string) error {
	if len(positionals) > 1 {
		return usage("invalid_input", "reveal accepts exactly one full share URL")
	}
	if len(positionals) == 1 && a.env.StdinPiped {
		return usage("invalid_input", "a share URL cannot be supplied both as an argument and through stdin")
	}
	if countSources(flags.ask, flags.passphraseFile != "", flags.passphraseFD != -1) > 1 {
		return usage("invalid_credential_source", "select exactly one passphrase source")
	}
	if a.cfg.json && flags.out != "" {
		return usage("invalid_option", "--json and --out are mutually exclusive")
	}
	var rawURL string
	if len(positionals) == 1 {
		rawURL = positionals[0]
		if err := a.warnBeforeNetwork("the share URL is now present in shell history"); err != nil {
			return err
		}
	} else if a.env.StdinPiped {
		data, err := readBounded(a.context(), a.env.Stdin, 4096)
		if err != nil {
			return err
		}
		rawURL = oneLine(data)
	} else {
		t, err := a.terminal()
		if err != nil {
			return usage("invalid_input", "a full share URL is required")
		}
		line, err := term.ReadLineContext(a.context(), t, "Share URL: ")
		if err != nil {
			if errors.Is(err, term.ErrInterrupted) || errors.Is(err, context.Canceled) {
				return interrupted(err)
			}
			if errors.Is(err, term.ErrInputTooLong) {
				return usage("invalid_input", "the share URL exceeds the supported size limit")
			}
			return local("cannot read the share URL from the terminal")
		}
		rawURL = line
	}
	target, err := id.ParseShareURL(rawURL)
	if err != nil {
		return usage("invalid_input", "reveal requires a full /s/<id> HTTP or HTTPS share URL")
	}
	server, err := api.ParseBaseURL(target.Origin)
	if err != nil {
		return usage("invalid_input", "the share URL does not name a safe server origin")
	}
	if a.cfg.serverFlag {
		if err := a.warnBeforeNetwork("--server is ignored for reveal; the share URL selects the server"); err != nil {
			return attachServer(err, server)
		}
	}
	phrase, err := a.readPassphrase(flags.ask, flags.passphraseFile, flags.passphraseFD)
	if err != nil {
		return attachServer(err, server)
	}
	defer func() { secret.Wipe(phrase) }()

	out, err := a.prepareDestination(flags.out)
	if err != nil {
		return attachServer(err, server)
	}
	if out != nil {
		defer out.discardUnlessWritten()
	}
	var recovery *reservedFile
	if flags.keepBlob != "" {
		recovery, err = reserve(flags.keepBlob)
		if err != nil {
			return attachServer(err, server)
		}
		defer recovery.discardUnlessWritten()
	}
	client, err := api.New(api.Config{BaseURL: server, Timeout: a.cfg.timeout, UserAgent: "burnerpad-cli/" + a.env.Version})
	if err != nil {
		return usage("invalid_input", "the share URL does not name a safe server origin")
	}
	if err := a.discloseServer(server); err != nil {
		return err
	}
	blob, err := client.Reveal(a.context(), target.ID)
	if err != nil {
		return mapAPIError(err, server)
	}
	defer secret.Wipe(blob)
	if recovery != nil {
		encoded := append(envelope.EncodeToBytes(blob), '\n')
		if err := recovery.write(encoded); err != nil {
			a.warn("the secret was claimed, but its recovery ciphertext could not be saved")
			return attachServer(err, server)
		}
		recovery.written = true
	}
	plaintext, err := envelope.DecryptPassphrase(blob, phrase)
	for err == envelope.ErrAuthFail && a.canRetryPassphrase() {
		secret.Wipe(phrase)
		fmt.Fprintln(a.env.Stderr, "burnerpad: the phrase did not open it; try again locally")
		phrase, err = a.readPassphrase(true, "", -1)
		if err != nil {
			break
		}
		plaintext, err = envelope.DecryptPassphrase(blob, phrase)
	}
	if err != nil {
		if recovery == nil {
			a.warn("the claimed ciphertext is being discarded; use --keep-blob before claiming when recovery may be needed")
		}
		return mapDecryptError(err, server)
	}
	defer secret.Wipe(plaintext)
	if !utf8.Valid(plaintext) {
		if recovery == nil {
			a.warn("the claimed ciphertext is being discarded")
		}
		return commandError{exit: 5, code: "plaintext_invalid", message: "the authenticated plaintext is not valid UTF-8 text", server: server}
	}
	if err := a.deliverPlaintext(out, "revealed", server, plaintext); err != nil {
		if recovery == nil {
			a.warn("the claimed ciphertext is being discarded")
		}
		return attachServer(err, server)
	}
	return nil
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

func (a *application) deliverPlaintext(out *reservedFile, status, server string, plaintext []byte) error {
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
		if status == "revealed" {
			if err := writeJSON(a.env.Stdout, revealResult{Status: status, Server: server, Plaintext: string(plaintext)}); err != nil {
				return local("cannot write JSON output")
			}
		} else {
			if err := writeJSON(a.env.Stdout, decryptResult{Status: status, Plaintext: string(plaintext)}); err != nil {
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

func (r *reservedFile) discardUnlessWritten() {
	if r != nil && !r.written {
		r.discard()
	}
}
