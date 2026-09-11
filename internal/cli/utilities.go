package cli

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"

	"github.com/burnerpad/burnerpad-cli/envelope"
	"github.com/burnerpad/burnerpad-cli/wordlist"
)

const wordsAttribution = "burnerpad words: EFF Short Wordlist #2 — Copyright (C) Electronic Frontier Foundation — CC BY 3.0 — https://www.eff.org/dice"

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
	if _, err := fmt.Fprintln(a.env.Stderr, wordsAttribution); err != nil {
		return local("cannot write wordlist attribution")
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
	var text string
	if len(positionals) == 0 {
		text = generalHelpText()
	} else {
		spec := findCommandSpec(positionals[0])
		if spec == nil {
			return usage("invalid_input", "unknown help topic")
		}
		text = commandHelpText(spec)
	}
	if _, err := fmt.Fprint(a.env.Stdout, text); err != nil {
		return local("cannot write help")
	}
	return nil
}

func generalHelpText() string {
	var text strings.Builder
	text.WriteString("burnerpad — encrypted, one-time text secrets\n\nUsage:\n  burnerpad COMMAND [OPTIONS]\n  burnerpad --help | -h\n  burnerpad --version\n\nCommands:\n")
	width := 0
	for _, spec := range commandSpecs {
		if len(spec.name) > width {
			width = len(spec.name)
		}
	}
	for _, spec := range commandSpecs {
		fmt.Fprintf(&text, "  %-*s  %s\n", width, spec.name, spec.description)
	}
	text.WriteString(`
Network commands disclose their selected server before sending one request.
Passphrases come only from protected files, descriptors 3 or greater, or the
controlling-terminal prompt. Management tokens use those protected sources or
a piped create receipt.

Run "burnerpad help COMMAND", "burnerpad COMMAND --help", or
"burnerpad COMMAND -h" for command-specific help.
`)
	return text.String()
}

func commandHelpText(spec *commandSpec) string {
	var text strings.Builder
	fmt.Fprintf(&text, "Usage:\n  burnerpad %s", spec.name)
	if spec.usage != "" {
		fmt.Fprintf(&text, " %s", spec.usage)
	}
	fmt.Fprintf(&text, "\n\n%s\n", spec.details)

	flags := optionsForCommand(spec)
	if len(flags) == 0 {
		return text.String()
	}
	labels := make([]string, len(flags))
	width := 0
	for i, option := range flags {
		labels[i] = option.name
		if option.takesValue {
			valueName := option.valueName
			if valueName == "" {
				valueName = "VALUE"
			}
			labels[i] += " " + valueName
		}
		if len(labels[i]) > width {
			width = len(labels[i])
		}
	}
	text.WriteString("\nOptions:\n")
	for i, option := range flags {
		fmt.Fprintf(&text, "  %-*s  %s\n", width, labels[i], option.description)
	}
	return text.String()
}
