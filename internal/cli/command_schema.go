package cli

import (
	"flag"
	"strings"
)

// commandSpec is the single command/option vocabulary. Runtime dispatch,
// help, and every completion adapter consume it, so a surface cannot merely
// guess which command or option exists.
type commandSpec struct {
	kind                 commandKind
	name                 string
	description          string
	usage                string
	details              string
	network              bool
	json                 bool
	terminalPresentation bool
	register             func(*flag.FlagSet) any
}

type commandKind uint8

type commandOption struct {
	name, valueName, description string
	takesValue                   bool
	fileValue                    bool
}

type boolFlag interface {
	IsBoolFlag() bool
}

const (
	commandCreate commandKind = iota
	commandReveal
	commandBurn
	commandDecrypt
	commandWords
	commandCompletion
	commandVersion
	commandLicenses
	commandHelp
)

var commandSpecs = []commandSpec{
	{
		kind: commandCreate, name: "create", description: "encrypt and store a text secret",
		network: true, json: true, terminalPresentation: true,
		usage: "[OPTIONS]",
		details: "Encrypt non-empty UTF-8 text locally and upload only opaque ciphertext. Plaintext comes from\n" +
			"the interactive composer, piped stdin, or --input. With no passphrase source, create generates a\n" +
			"fresh seven-word phrase.",
		register: func(fs *flag.FlagSet) any { return registerCreate(fs) },
	},
	{
		kind: commandReveal, name: "reveal", description: "claim and decrypt a full share URL",
		network: true, json: true, terminalPresentation: true,
		usage: "[OPTIONS] [FULL_SHARE_URL]",
		details: "Claim one secret exactly once, then decrypt it locally. Supply a full /s/<id> URL in argv, pipe\n" +
			"only the URL, or omit it for a protected terminal prompt. Credentials and destinations are checked\n" +
			"before the claim; --keep-blob preserves recovery ciphertext after it.",
		register: func(fs *flag.FlagSet) any { return registerReveal(fs) },
	},
	{
		kind: commandBurn, name: "burn", description: "revoke without revealing", network: true, json: true,
		usage: "[OPTIONS] [FULL_SHARE_URL|ID]",
		details: "Revoke without retrieving ciphertext. Pipe a create receipt, or supply a full share URL or bare\n" +
			"current ID with a protected management-token source. A URL or receipt selects its own server; a bare\n" +
			"ID uses --server, BURNERPAD_SERVER, or the default.",
		register: func(fs *flag.FlagSet) any { return registerBurn(fs) },
	},
	{
		kind: commandDecrypt, name: "decrypt", description: "decrypt a preserved blob offline",
		json: true, terminalPresentation: true,
		usage: "[OPTIONS] --blob-file FILE|-",
		details: "Decrypt canonical recovery ciphertext locally without constructing an HTTP client. Plaintext goes\n" +
			"to a new --out file, stable JSON, a terminal-safe viewer, or exact piped stdout.",
		register: func(fs *flag.FlagSet) any { return registerDecrypt(fs) },
	},
	{
		kind: commandWords, name: "words", description: "print the shared wordlist",
		details: "Write the EFF copyright and CC BY 3.0 attribution to stderr, then print the embedded EFF\n" +
			"Short Wordlist #2 to stdout, one word per line, in canonical order.",
	},
	{
		kind: commandCompletion, name: "completion", description: "print a shell completion",
		usage: "bash|zsh|fish|powershell", details: "Print a sourceable completion script for exactly one supported shell.",
	},
	{
		kind: commandVersion, name: "version", description: "print build and protocol identity",
		details: "Print build metadata, exact envelope/vector/wordlist identities, and the server compatibility summary.",
	},
	{
		kind: commandLicenses, name: "licenses", description: "print license notices",
		details: "Print the license and attribution notices embedded in the executable.",
	},
	{
		kind: commandHelp, name: "help", description: "print help", usage: "[COMMAND]",
		details: "Print general help, or detailed help for one command.",
	},
}

func findCommandSpec(name string) *commandSpec {
	for i := range commandSpecs {
		if commandSpecs[i].name == name {
			return &commandSpecs[i]
		}
	}
	return nil
}

func optionsFromFlagSet(fs *flag.FlagSet) []commandOption {
	var options []commandOption
	fs.VisitAll(func(f *flag.Flag) {
		isBool := false
		if value, ok := f.Value.(boolFlag); ok {
			isBool = value.IsBoolFlag()
		}
		valueName, description := flag.UnquoteUsage(f)
		usage := strings.ToLower(description)
		options = append(options, commandOption{
			name:        "--" + f.Name,
			valueName:   valueName,
			description: description,
			takesValue:  !isBool,
			fileValue:   !isBool && strings.Contains(usage, "file") && !strings.Contains(usage, "descriptor"),
		})
	})
	return options
}

func optionsForCommand(spec *commandSpec) []commandOption {
	fs := flag.NewFlagSet(spec.name, flag.ContinueOnError)
	var globals globalFlags
	registerCommandFlags(fs, spec, &globals)
	options := optionsFromFlagSet(fs)
	for i := range options {
		if options[i].name != "--server" {
			continue
		}
		switch spec.kind {
		case commandReveal:
			options[i].valueName = "IGNORED"
			options[i].description = "ignored; the share URL selects its server"
		case commandBurn:
			options[i].description = "select the server for a bare ID; ignored for a URL or receipt"
		}
	}
	return options
}

func registerCommandFlags(fs *flag.FlagSet, spec *commandSpec, globals *globalFlags) any {
	if spec.network {
		registerNetworkFlags(fs, globals)
	}
	if spec.json {
		registerJSONFlag(fs, globals)
	}
	if spec.terminalPresentation {
		registerTerminalPresentationFlags(fs, globals)
	}
	if spec.register == nil {
		return nil
	}
	return spec.register(fs)
}

func runCommand(spec *commandSpec, a *application, flags any, args []string) error {
	switch spec.kind {
	case commandCreate:
		return runCreate(a, flags.(*createFlags), args)
	case commandReveal:
		return runReveal(a, flags.(*revealFlags), args)
	case commandBurn:
		return runBurn(a, flags.(*burnFlags), args)
	case commandDecrypt:
		return runDecrypt(a, flags.(*decryptFlags), args)
	case commandWords:
		return runWords(a, args)
	case commandCompletion:
		return runCompletion(a, args)
	case commandVersion:
		return runVersion(a, args)
	case commandLicenses:
		return runLicenses(a, args)
	case commandHelp:
		return runHelp(a, args)
	default:
		return usage("invalid_command", "unknown command")
	}
}
