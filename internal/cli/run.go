package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/burnerpad/burnerpad-cli/internal/term"
)

const defaultServer = "https://burnerpad.io"

type config struct {
	server      string
	serverFlag  bool
	timeoutFlag bool
	jsonFlag    bool
	quietFlag   bool
	plainFlag   bool
	noColorFlag bool
	timeout     time.Duration
	json        bool
	quiet       bool
	plain       bool
	noColor     bool
}

type application struct {
	env Env
	cfg config
	tty *term.TTY
}

type commandError struct {
	exit       int
	code       string
	message    string
	server     string
	retryAfter *int64
}

func (e commandError) Error() string { return e.message }

func usage(code, message string) error {
	return commandError{exit: 2, code: code, message: message}
}

func local(message string) error {
	return commandError{exit: 3, code: "local_io_failed", message: message}
}

func Run(env Env) (exit int) {
	if env.Stdin == nil {
		env.Stdin = strings.NewReader("")
	}
	if env.Stdout == nil {
		env.Stdout = io.Discard
	}
	if env.Stderr == nil {
		env.Stderr = io.Discard
	}
	if env.Getenv == nil {
		env.Getenv = func(string) string { return "" }
	}
	a := &application{env: env}
	for _, arg := range env.Args {
		if arg == "--json" || arg == "--json=true" {
			a.cfg.json = true
		}
	}
	defer func() {
		if a.tty != nil {
			a.tty.EmergencyRestore()
			a.tty.Close()
		}
		if recovered := recover(); recovered != nil {
			fmt.Fprintln(env.Stderr, "burnerpad: internal: unexpected internal failure")
			exit = 10
		}
	}()

	signals := env.Signals
	var stop func()
	if signals == nil {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		signals = ch
		stop = func() { signal.Stop(ch) }
		defer stop()
	}
	done := make(chan error, 1)
	go func() {
		defer func() {
			if recover() != nil {
				done <- commandError{exit: 10, code: "internal", message: "unexpected internal failure"}
			}
		}()
		done <- dispatch(a)
	}()
	select {
	case sig := <-signals:
		if sig == syscall.SIGTERM {
			return 143
		}
		return 130
	case err := <-done:
		return a.report(err)
	}
}

func (a *application) report(err error) int {
	if err == nil {
		return 0
	}
	var ce commandError
	if !errors.As(err, &ce) {
		ce = commandError{exit: 10, code: "internal", message: "unexpected internal failure"}
	}
	if ce.exit >= 128 {
		return ce.exit
	}
	if a.cfg.json {
		body := errorResult{Status: "error", Code: ce.code, Message: ce.message, Server: ce.server, RetryAfter: ce.retryAfter}
		if writeJSON(a.env.Stdout, body) != nil {
			fmt.Fprintln(a.env.Stderr, "burnerpad: local_io_failed: cannot write JSON output")
			return 3
		}
		return ce.exit
	}
	fmt.Fprintf(a.env.Stderr, "burnerpad: %s: %s\n", ce.code, ce.message)
	return ce.exit
}

var commands = map[string]bool{
	"create": true, "reveal": true, "burn": true, "decrypt": true,
	"words": true, "completion": true, "version": true, "licenses": true, "help": true,
}

func dispatch(a *application) error {
	name, args, err := splitCommand(a.env.Args)
	if err != nil {
		return err
	}
	if name == "" {
		return usage("invalid_command", "a command is required; run 'burnerpad help'")
	}
	if !commands[name] {
		return usage("invalid_command", "unknown command; run 'burnerpad help'")
	}
	if duplicateOption(args) {
		return usage("invalid_option", "an option was supplied more than once")
	}

	g := globalFlags{timeout: 12 * time.Second}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	registerGlobals(fs, &g)
	var command any
	switch name {
	case "create":
		command = registerCreate(fs)
	case "reveal":
		command = registerReveal(fs)
	case "burn":
		command = registerBurn(fs)
	case "decrypt":
		command = registerDecrypt(fs)
	}
	positionals, err := parseInterleaved(fs, args)
	if err != nil {
		return usage("invalid_option", safeFlagError(err))
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "server":
			g.serverExplicit = true
		case "timeout":
			a.cfg.timeoutFlag = true
		case "json":
			a.cfg.jsonFlag = true
		case "quiet":
			a.cfg.quietFlag = true
		case "plain":
			a.cfg.plainFlag = true
		case "no-color":
			a.cfg.noColorFlag = true
		}
	})
	explicit := a.cfg
	a.cfg = resolveConfig(a.env, g)
	a.cfg.timeoutFlag = explicit.timeoutFlag
	a.cfg.jsonFlag = explicit.jsonFlag
	a.cfg.quietFlag = explicit.quietFlag
	a.cfg.plainFlag = explicit.plainFlag
	a.cfg.noColorFlag = explicit.noColorFlag
	if a.cfg.timeout <= 0 {
		return usage("invalid_option", "--timeout must be positive")
	}

	switch name {
	case "create":
		return runCreate(a, command.(*createFlags), positionals)
	case "reveal":
		return runReveal(a, command.(*revealFlags), positionals)
	case "burn":
		return runBurn(a, command.(*burnFlags), positionals)
	case "decrypt":
		return runDecrypt(a, command.(*decryptFlags), positionals)
	case "words":
		return runWords(a, positionals)
	case "completion":
		return runCompletion(a, positionals)
	case "version":
		return runVersion(a, positionals)
	case "licenses":
		return runLicenses(a, positionals)
	case "help":
		return runHelp(a, positionals)
	}
	return usage("invalid_command", "unknown command")
}

func duplicateOption(args []string) bool {
	seen := make(map[string]bool)
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		name := strings.TrimPrefix(arg, "--")
		if at := strings.IndexByte(name, '='); at >= 0 {
			name = name[:at]
		}
		if seen[name] {
			return true
		}
		seen[name] = true
	}
	return false
}

func splitCommand(args []string) (string, []string, error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if commands[arg] {
			out := append([]string{}, args[:i]...)
			out = append(out, args[i+1:]...)
			return arg, out, nil
		}
		if arg == "--version" {
			return "version", append([]string{}, args[:i]...), nil
		}
		if arg == "--help" || arg == "-h" {
			return "help", nil, nil
		}
		if arg == "--server" || arg == "--timeout" {
			i++
			if i >= len(args) {
				return "", nil, usage("invalid_option", "option needs a value")
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return "", nil, usage("invalid_command", "unknown command; run 'burnerpad help'")
	}
	return "", nil, nil
}

func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positionals []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positionals, nil
		}
		positionals = append(positionals, rest[0])
		args = rest[1:]
	}
}

func safeFlagError(err error) string {
	if strings.Contains(err.Error(), "passphrase") {
		return "passphrases cannot be supplied on the command line; use a protected file, fd, or terminal prompt"
	}
	if strings.Contains(err.Error(), "token") {
		return "management tokens cannot be supplied on the command line; use a protected file, fd, or terminal prompt"
	}
	return "invalid or unsupported option"
}

func (a *application) terminal() (*term.TTY, error) {
	if a.tty != nil {
		return a.tty, nil
	}
	if a.env.OpenTTY == nil {
		return nil, local("no controlling terminal is available")
	}
	t, err := a.env.OpenTTY()
	if err != nil {
		return nil, local("no controlling terminal is available")
	}
	a.tty = t
	return t, nil
}

// canRetryPassphrase reports whether a claimed blob can be retried against a
// corrected phrase locally. Opening the controlling terminal is harmless and
// does not perform another network operation.
func (a *application) canRetryPassphrase() bool {
	_, err := a.terminal()
	return err == nil
}
