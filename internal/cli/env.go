// Package cli is the command chassis: flag parsing and dispatch, stream
// discipline, the single exit table, configuration and credential-source
// resolution, and the small words/licenses/version/completion/help commands.
// Network and local subcommands live in their own files.
//
// User-visible behavior is specified by docs/ARCHITECTURE.md. Stable machine
// formats and command-schema relationships are exercised as contracts.
package cli

import (
	"io"
	"os"

	"github.com/burnerpad/burnerpad-cli/internal/term"
	xterm "golang.org/x/term"
)

// Env is the complete process environment as a value: argv, streams,
// TTY-ness, env lookup, the controlling terminal, signals, and build
// identity. cli.Run is a pure function of an Env (design invariant 6), which
// is what makes the whole CLI drivable from tests with no subprocess and no
// real terminal.
type Env struct {
	Args []string // argv WITHOUT the program name

	Stdin          io.Reader
	Stdout, Stderr io.Writer

	// TTY-ness per stream, decided by term.IsTerminal on the real descriptors;
	// environment variables never decide interactivity.
	StdinTTY, StdoutTTY, StderrTTY bool
	// StdinPiped distinguishes a real pipe/redirect from a non-interactive
	// character device such as /dev/null when checking competing inputs.
	StdinPiped bool

	// Getenv looks up one environment variable ("" = unset). The complete
	// variable surface is deliberately closed; nothing else is ever read.
	Getenv func(string) string

	// OpenTTY opens the controlling terminal (/dev/tty; CONIN$/CONOUT$ on
	// Windows) for prompts that must survive stdio redirection.
	OpenTTY func() (*term.TTY, error)

	// Signals delivers termination signals. nil ⇒ Run installs
	// signal.Notify(SIGINT, SIGTERM) itself; tests supply their
	// own channel.
	Signals <-chan os.Signal

	// Build identity comes from -ldflags -X main.*. Date is the commit date,
	// never the build clock.
	Version, Commit, Date string
}

// OSEnv builds the real-process Env: os.Args/stdio/os.Getenv, TTY-ness via
// x/term.IsTerminal, the controlling terminal via term.OpenTTY, and nil
// Signals so Run installs its own handler.
func OSEnv(version, commit, date string) Env {
	stdinTTY := xterm.IsTerminal(int(os.Stdin.Fd()))
	stdinPiped := false
	if info, err := os.Stdin.Stat(); err == nil {
		stdinPiped = !stdinTTY && info.Mode()&os.ModeCharDevice == 0
	}
	return Env{
		Args:       os.Args[1:],
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		StdinTTY:   stdinTTY,
		StdinPiped: stdinPiped,
		StdoutTTY:  xterm.IsTerminal(int(os.Stdout.Fd())),
		StderrTTY:  xterm.IsTerminal(int(os.Stderr.Fd())),
		Getenv:     os.Getenv,
		OpenTTY:    term.OpenTTY,
		Signals:    nil,
		Version:    version,
		Commit:     commit,
		Date:       date,
	}
}
