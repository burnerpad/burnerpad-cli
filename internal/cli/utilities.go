package cli

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"

	"github.com/burnerpad/burnerpad-cli/envelope"
	"github.com/burnerpad/burnerpad-cli/wordlist"
)

func utilityArgs(name string, positionals []string) error {
	if len(positionals) != 0 {
		return usage("invalid_input", name+" does not accept arguments")
	}
	return nil
}

func runWords(a *application, positionals []string) error {
	if err := utilityArgs("words", positionals); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(a.env.Stdout, strings.Join(wordlist.Words(), "\n")); err != nil {
		return local("cannot write word list")
	}
	return nil
}

func runVersion(a *application, positionals []string) error {
	if err := utilityArgs("version", positionals); err != nil {
		return err
	}
	_, err := fmt.Fprintf(a.env.Stdout,
		"burnerpad %s (commit %s, %s, %s)\nenvelope: suite 0x02, %s, vectors sha256:%s\nwordlist: EFF Short Wordlist #2 (%d words, sha256:%s)\nserver-api: current burnerpad-lite (default %s)\n",
		a.env.Version, a.env.Commit, runtime.Version(), a.env.Date, envelope.SpecVersion, envelope.VectorsSHA256,
		wordlist.Count, wordlist.FileSHA256, defaultServer)
	if err != nil {
		return local("cannot write version information")
	}
	return nil
}

//go:embed notice.txt
var noticeText []byte

func runLicenses(a *application, positionals []string) error {
	if err := utilityArgs("licenses", positionals); err != nil {
		return err
	}
	if _, err := a.env.Stdout.Write(noticeText); err != nil {
		return local("cannot write license information")
	}
	return nil
}

func runHelp(a *application, positionals []string) error {
	if len(positionals) > 1 {
		return usage("invalid_input", "help accepts at most one command")
	}
	text := `burnerpad — encrypted, one-time text secrets

Usage:
  burnerpad create [--server ORIGIN] [--ttl DURATION] [--input FILE]
  burnerpad reveal [options] FULL_SHARE_URL
  burnerpad burn [options] [FULL_SHARE_URL|ID]
  burnerpad decrypt --blob-file FILE|-
  burnerpad words
  burnerpad completion bash|zsh|fish|powershell
  burnerpad version
  burnerpad licenses
  burnerpad help [COMMAND]

Network commands disclose their selected server before sending one request.
Passphrases and management tokens are accepted only from protected files,
descriptors 3 or greater, or the controlling-terminal prompt.

Run "burnerpad help create", "burnerpad help reveal", "burnerpad help burn",
or "burnerpad help decrypt" for command options.
`
	if len(positionals) == 1 {
		switch positionals[0] {
		case "create":
			text = "Usage: burnerpad create [--server ORIGIN] [--timeout DURATION] [--json] [--plain] [--no-color] [--ttl DURATION] [--input FILE] [--ask|--passphrase-file FILE|--passphrase-fd FD]\n"
		case "reveal":
			text = "Usage: burnerpad reveal [--server IGNORED] [--timeout DURATION] [--json] [--plain] [--no-color] [--ask|--passphrase-file FILE|--passphrase-fd FD] [--keep-blob FILE] [--out FILE] FULL_SHARE_URL\n"
		case "burn":
			text = "Usage: burnerpad burn [--server ORIGIN] [--timeout DURATION] [--json] [--plain] [--no-color] [--token-file FILE|--token-fd FD] [FULL_SHARE_URL|ID]\nA piped create receipt supplies the link, server, and management token.\n"
		case "decrypt":
			text = "Usage: burnerpad decrypt [--json] [--plain] [--no-color] --blob-file FILE|- [--ask|--passphrase-file FILE|--passphrase-fd FD] [--out FILE]\n"
		default:
			return usage("invalid_input", "unknown help topic")
		}
	}
	if _, err := fmt.Fprint(a.env.Stdout, text); err != nil {
		return local("cannot write help")
	}
	return nil
}
