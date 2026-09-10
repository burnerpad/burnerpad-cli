package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/burnerpad/burnerpad-cli/envelope"
	"github.com/burnerpad/burnerpad-cli/internal/api"
	"github.com/burnerpad/burnerpad-cli/internal/secret"
	"github.com/burnerpad/burnerpad-cli/internal/term"
	"github.com/burnerpad/burnerpad-cli/wordlist"
)

const maxPlaintext = 65_491

func runCreate(a *application, flags *createFlags, positionals []string) error {
	if len(positionals) != 0 {
		return usage("invalid_input", "create does not accept positional arguments")
	}
	if flags.input == "-" {
		return usage("invalid_option", "--input requires a named file")
	}
	if flags.input != "" && a.env.StdinPiped {
		return usage("invalid_input", "--input and piped stdin are ambiguous plaintext sources")
	}
	if countSources(flags.ask, flags.passphraseFile != "", flags.passphraseFD != -1) > 1 {
		return usage("invalid_credential_source", "select exactly one passphrase source")
	}
	if flags.passphraseFile == "-" || (flags.passphraseFD != -1 && flags.passphraseFD < 3) {
		return usage("invalid_credential_source", "passphrase files must be named and passphrase descriptors must be 3 or greater")
	}
	ttl, err := parseTTL(flags.ttl)
	if err != nil {
		return err
	}
	server, err := api.ParseBaseURL(a.cfg.server)
	if err != nil {
		return usage("invalid_input", "the selected server is not a safe origin")
	}
	var clipOut io.Writer
	if flags.clip {
		clipOut, err = a.clipboardWriter()
		if err != nil {
			return attachServer(err, server)
		}
	}
	plaintext, err := a.readCreatePlaintext(flags.input)
	if err != nil {
		return attachServer(err, server)
	}
	defer secret.Wipe(plaintext)

	phrase, supplied, err := a.createPhrase(flags)
	if err != nil {
		return attachServer(err, server)
	}
	defer secret.Wipe(phrase)
	if !supplied && !a.env.StdinTTY && !a.cfg.json {
		return attachServer(usage("invalid_option", "a piped create with a generated passphrase requires --json"), server)
	}
	blob := envelope.EncryptPassphrase(phrase, plaintext)
	defer secret.Wipe(blob)
	client, err := api.New(api.Config{BaseURL: server, Timeout: a.cfg.timeout, UserAgent: "burnerpad-cli/" + a.env.Version})
	if err != nil {
		return attachServer(usage("invalid_input", "the selected server is not a safe origin"), server)
	}
	if err := a.discloseServer(server); err != nil {
		return err
	}
	created, err := client.Create(a.context(), blob, ttl)
	if err != nil {
		return mapAPIError(err, server)
	}
	link := server + "/s/" + created.ID
	if a.cfg.json {
		if err := writeJSON(a.env.Stdout, createResult{Status: "created", Server: server, Link: link, Phrase: string(phrase), MgmtToken: created.MgmtToken, TTL: created.TTL}); err != nil {
			return attachServer(local("cannot write JSON output"), server)
		}
	} else {
		if _, err := fmt.Fprintln(a.env.Stdout, link); err != nil {
			return attachServer(local("cannot write the share link"), server)
		}
		if !supplied || a.env.StdinTTY {
			if _, err := fmt.Fprintf(a.env.Stderr, "burnerpad: passphrase: %s\n", phrase); err != nil {
				return attachServer(local("cannot write the create handoff"), server)
			}
		}
		if _, err := fmt.Fprintf(a.env.Stderr, "burnerpad: management token: %s\n", created.MgmtToken); err != nil {
			return attachServer(local("cannot write the create handoff"), server)
		}
		if _, err := fmt.Fprintf(a.env.Stderr, "burnerpad: effective ttl: %s\n", durationLabel(created.TTL)); err != nil {
			return attachServer(local("cannot write the create handoff"), server)
		}
		if ttl != nil && *ttl != created.TTL {
			if _, err := fmt.Fprintf(a.env.Stderr, "burnerpad: note: requested ttl was clamped to %s\n", durationLabel(created.TTL)); err != nil {
				return attachServer(local("cannot write the create handoff"), server)
			}
		}
	}
	if flags.clip {
		if err := term.OSC52Copy(clipOut, []byte(link)); err != nil {
			return attachServer(local("cannot write the OSC 52 clipboard sequence"), server)
		}
	}
	return nil
}

func (a *application) readCreatePlaintext(path string) ([]byte, error) {
	var (
		data []byte
		err  error
	)
	switch {
	case path != "":
		f, openErr := os.Open(path)
		if openErr != nil {
			return nil, local("cannot read the plaintext input file")
		}
		defer f.Close()
		data, err = readBounded(a.context(), f, maxPlaintext)
	case !a.env.StdinTTY:
		data, err = readBounded(a.context(), a.env.Stdin, maxPlaintext)
	default:
		fmt.Fprintln(a.env.Stderr, "Secret — type or paste text, then finish with EOF:")
		data, err = readBounded(a.context(), a.env.Stdin, maxPlaintext)
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, usage("invalid_input", "the plaintext must not be empty")
	}
	if !utf8.Valid(data) {
		return nil, usage("invalid_input", "the plaintext must be valid UTF-8 text")
	}
	return data, nil
}

func readBounded(ctx context.Context, reader io.Reader, limit int) ([]byte, error) {
	read := func() ([]byte, error) {
		return io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	}
	var data []byte
	var err error
	if closer, ok := reader.(io.Closer); ok {
		stopClose := context.AfterFunc(ctx, func() { _ = closer.Close() })
		data, err = read()
		stopClose()
	} else {
		type result struct {
			data []byte
			err  error
		}
		resultC := make(chan result)
		go func() {
			got, readErr := read()
			select {
			case resultC <- result{data: got, err: readErr}:
			case <-ctx.Done():
				secret.Wipe(got)
			}
		}()
		select {
		case got := <-resultC:
			data, err = got.data, got.err
		case <-ctx.Done():
			return nil, localCause("cannot read input", ctx.Err())
		}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		secret.Wipe(data)
		return nil, localCause("cannot read input", ctxErr)
	}
	if err != nil {
		secret.Wipe(data)
		return nil, local("cannot read input")
	}
	if len(data) > limit {
		secret.Wipe(data)
		return nil, usage("invalid_input", "input exceeds the supported size limit")
	}
	return data, nil
}

func (a *application) createPhrase(flags *createFlags) ([]byte, bool, error) {
	if flags.ask || flags.passphraseFile != "" || flags.passphraseFD != -1 {
		phrase, err := a.readPassphrase(flags.ask, flags.passphraseFile, flags.passphraseFD)
		return phrase, true, err
	}
	return wordlist.Phrase(), false, nil
}

func (a *application) readPassphrase(ask bool, path string, descriptor int) ([]byte, error) {
	if countSources(ask, path != "", descriptor != -1) > 1 {
		return nil, usage("invalid_credential_source", "select exactly one passphrase source")
	}
	var raw []byte
	var err error
	switch {
	case path != "":
		if path == "-" {
			return nil, usage("invalid_credential_source", "--passphrase-file does not accept -")
		}
		f, openErr := os.Open(path)
		if openErr != nil {
			return nil, local("cannot read the passphrase file")
		}
		raw, err = readBounded(a.context(), f, wordlist.MaxPhraseBytes)
		f.Close()
	case descriptor != -1:
		if descriptor < 3 {
			return nil, usage("invalid_credential_source", "--passphrase-fd must be 3 or greater")
		}
		f := os.NewFile(uintptr(descriptor), "passphrase-fd")
		if f == nil {
			return nil, local("cannot read the passphrase descriptor")
		}
		raw, err = readBounded(a.context(), f, wordlist.MaxPhraseBytes)
		f.Close()
	default:
		t, ttyErr := a.terminal()
		if ttyErr != nil {
			return nil, usage("invalid_credential_source", "a protected passphrase source is required")
		}
		buffer, promptErr := term.ReadPhraseContext(a.context(), t, term.PhraseOpts{Plain: a.cfg.plain, NoColor: a.cfg.noColor})
		if promptErr != nil {
			if errors.Is(promptErr, term.ErrInterrupted) || errors.Is(promptErr, context.Canceled) {
				return nil, interrupted(promptErr)
			}
			return nil, local("cannot read the passphrase from the terminal")
		}
		raw = append([]byte(nil), buffer.Bytes()...)
		buffer.Wipe()
	}
	if err != nil {
		return nil, err
	}
	canonical, err := wordlist.Canonicalize(raw)
	secret.Wipe(raw)
	if err != nil {
		return nil, usage("invalid_credential_source", "the passphrase must contain 7 to 64 distinct Burnerpad words")
	}
	return canonical, nil
}

func parseTTL(raw string) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if n <= 0 {
			return nil, usage("invalid_input", "--ttl must be positive")
		}
		return &n, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 || d%time.Second != 0 {
		return nil, usage("invalid_input", "--ttl must be a positive whole number of seconds")
	}
	n := int64(d / time.Second)
	return &n, nil
}

func durationLabel(seconds int64) string {
	if seconds%3600 == 0 {
		return fmt.Sprintf("%dh", seconds/3600)
	}
	if seconds%60 == 0 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	return fmt.Sprintf("%ds", seconds)
}
