package cli

import "flag"

// commandSpec is the single command/option vocabulary. Runtime dispatch and
// every completion adapter both consume it, so an option cannot be accepted
// by one surface and merely guessed by another.
type commandSpec struct {
	kind         commandKind
	name         string
	description  string
	network      bool
	presentation bool
	register     func(*flag.FlagSet) any
}

type commandKind uint8

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
		kind: commandCreate, name: "create", description: "encrypt and store a text secret", network: true, presentation: true,
		register: func(fs *flag.FlagSet) any { return registerCreate(fs) },
	},
	{
		kind: commandReveal, name: "reveal", description: "claim and decrypt a full share URL", network: true, presentation: true,
		register: func(fs *flag.FlagSet) any { return registerReveal(fs) },
	},
	{
		kind: commandBurn, name: "burn", description: "revoke without revealing", network: true, presentation: true,
		register: func(fs *flag.FlagSet) any { return registerBurn(fs) },
	},
	{
		kind: commandDecrypt, name: "decrypt", description: "decrypt a preserved blob offline", presentation: true,
		register: func(fs *flag.FlagSet) any { return registerDecrypt(fs) },
	},
	{kind: commandWords, name: "words", description: "print the shared wordlist"},
	{kind: commandCompletion, name: "completion", description: "print a shell completion"},
	{kind: commandVersion, name: "version", description: "print build and protocol identity"},
	{kind: commandLicenses, name: "licenses", description: "print license notices"},
	{kind: commandHelp, name: "help", description: "print help"},
}

func findCommandSpec(name string) *commandSpec {
	for i := range commandSpecs {
		if commandSpecs[i].name == name {
			return &commandSpecs[i]
		}
	}
	return nil
}

func registerCommandFlags(fs *flag.FlagSet, spec *commandSpec, globals *globalFlags) any {
	if spec.network {
		registerNetworkFlags(fs, globals)
	}
	if spec.presentation {
		registerPresentationFlags(fs, globals)
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
