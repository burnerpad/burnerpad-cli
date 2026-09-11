// Command burnerpad is the official CLI for burnerpad.io and self-hosted
// instances: end-to-end encrypted one-time secrets, created and revealed
// entirely client-side. See docs/ARCHITECTURE.md — every behavior here is
// specified there.
package main

import (
	"os"

	"github.com/burnerpad/burnerpad-cli/internal/cli"
	"github.com/burnerpad/burnerpad-cli/internal/secret"
)

// Set via -ldflags "-X main.version=… -X main.commit=… -X main.date=…". They
// must live in package main because the linker silently ignores -X for absent
// symbols. date is the commit date, never the build clock.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	// Harden before any secret exists: disable core dumps and, where supported,
	// same-user process attachment.
	// A hardened process is preferred, a working one is required.
	secret.Harden()
	os.Exit(cli.Run(cli.OSEnv(version, commit, date)))
}
