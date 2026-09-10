package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
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
	if countSources(a.cfg.json, flags.out != "", flags.clip.enabled) > 1 {
		return usage("invalid_option", "--json, --out, and --clip are mutually exclusive")
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

	destination, err := a.prepareDestination(flags.out, flags.clip)
	if err != nil {
		return attachServer(err, server)
	}
	if destination.out != nil {
		defer destination.discardUnlessWritten()
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
	if err := a.deliverPlaintext(destination, "revealed", server, plaintext); err != nil {
		if recovery == nil {
			a.warn("the claimed ciphertext is being discarded")
		}
		return attachServer(err, server)
	}
	return nil
}

type destination struct {
	out        *reservedFile
	clip       clipFlag
	clipWriter io.Writer
}

func (a *application) prepareDestination(path string, clip clipFlag) (*destination, error) {
	d := &destination{clip: clip}
	var err error
	if path != "" {
		d.out, err = reserve(path)
		if err != nil {
			return nil, err
		}
	}
	if clip.enabled {
		d.clipWriter, err = a.clipboardWriter()
		if err != nil {
			if d.out != nil {
				d.out.discard()
			}
			return nil, err
		}
	}
	if !a.cfg.json && d.out == nil && !d.clip.enabled && a.env.StdoutTTY {
		if _, err = a.terminal(); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func (a *application) clipboardWriter() (io.Writer, error) {
	if a.env.StderrTTY {
		return a.env.Stderr, nil
	}
	t, err := a.terminal()
	if err != nil {
		return nil, local("OSC 52 clipboard delivery needs a controlling terminal")
	}
	return t.Out(), nil
}

func (a *application) deliverPlaintext(d *destination, status, server string, plaintext []byte) error {
	// Once plaintext is ready—especially after a one-time claim—a signal that
	// arrived during local decryption must not erase the destination as soon as
	// it is entered. Complete that already-pending handoff: clipboard delivery
	// gets its configured dwell, while the viewer uses persistent plain output
	// instead of waiting on the alternate screen. When delivery begins before
	// cancellation, the Run context still cancels its interactive wait.
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
	case d.out != nil:
		if err := d.out.write(plaintext); err != nil {
			return err
		}
		d.out.written = true
	case d.clip.enabled:
		if err := term.OSC52Copy(d.clipWriter, plaintext); err != nil {
			return local("cannot write the OSC 52 clipboard sequence")
		}
		fmt.Fprintf(a.env.Stderr, "burnerpad: plaintext copied via OSC 52; best-effort clear in %s (clipboard managers may retain history)\n", d.clip.duration)
		timer := time.NewTimer(d.clip.duration)
		var canceled error
		select {
		case <-timer.C:
		case <-deliveryCtx.Done():
			canceled = deliveryCtx.Err()
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		if err := term.OSC52Clear(d.clipWriter); err != nil {
			a.warn("the best-effort clipboard clear sequence could not be written")
		}
		if canceled != nil {
			return interrupted(canceled)
		}
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

func (d *destination) discardUnlessWritten() {
	if d != nil && d.out != nil && !d.out.written {
		d.out.discard()
	}
}

func (r *reservedFile) discardUnlessWritten() {
	if r != nil && !r.written {
		r.discard()
	}
}
