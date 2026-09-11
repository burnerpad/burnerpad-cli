package cli

import (
	"os"
	"unicode/utf8"

	"github.com/burnerpad/burnerpad-cli/envelope"
	"github.com/burnerpad/burnerpad-cli/internal/secret"
)

func runDecrypt(a *application, flags *decryptFlags, positionals []string) error {
	if len(positionals) != 0 {
		return usage("invalid_input", "decrypt does not accept positional arguments")
	}
	if flags.blobFile == "" {
		return usage("invalid_input", "decrypt requires --blob-file FILE or --blob-file -")
	}
	if countSources(flags.ask, flags.passphraseFile != "", flags.passphraseFD != -1) > 1 {
		return usage("invalid_credential_source", "select exactly one passphrase source")
	}
	if a.cfg.json && flags.out != "" {
		return usage("invalid_option", "--json and --out are mutually exclusive")
	}
	out, err := a.prepareDestination(flags.out)
	if err != nil {
		return err
	}
	if out != nil {
		defer out.discardUnlessWritten()
	}
	phrase, err := a.readPassphrase(flags.ask, flags.passphraseFile, flags.passphraseFD)
	if err != nil {
		return err
	}
	defer func() { secret.Wipe(phrase) }()
	var encoded []byte
	if flags.blobFile == "-" {
		encoded, err = readBounded(a.context(), a.env.Stdin, 100_000)
	} else {
		f, openErr := os.Open(flags.blobFile)
		if openErr != nil {
			return local("cannot read the ciphertext file")
		}
		encoded, err = readBounded(a.context(), f, 100_000)
		f.Close()
	}
	if err != nil {
		return err
	}
	encoded = trimOneNewline(encoded)
	blob, err := envelope.DecodeCanonical(encoded)
	secret.Wipe(encoded)
	if err != nil {
		return commandError{exit: 8, code: "unsupported_secret", message: "the ciphertext file is not canonical Burnerpad base64url"}
	}
	defer secret.Wipe(blob)
	plaintext, err := envelope.DecryptPassphrase(blob, phrase)
	for err == envelope.ErrAuthFail && a.canRetryPassphrase() {
		secret.Wipe(phrase)
		phrase, err = a.readPassphrase(true, "", -1)
		if err != nil {
			break
		}
		plaintext, err = envelope.DecryptPassphrase(blob, phrase)
	}
	if err != nil {
		return mapDecryptError(err, "")
	}
	defer secret.Wipe(plaintext)
	if !utf8.Valid(plaintext) {
		return commandError{exit: 5, code: "plaintext_invalid", message: "the authenticated plaintext is not valid UTF-8 text"}
	}
	return a.deliverPlaintext(out, "decrypted", "", plaintext)
}
