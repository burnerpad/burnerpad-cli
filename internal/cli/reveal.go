package cli

import (
	"context"
	"errors"

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
	return a.decryptAndDeliver(blob, phrase, plaintextDelivery{
		out: out, server: server, claimedWithoutRecovery: recovery == nil, retry: a,
	})
}
