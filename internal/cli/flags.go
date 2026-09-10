package cli

import (
	"flag"
	"fmt"
	"strings"
	"time"
)

type globalFlags struct {
	server         string
	serverExplicit bool
	timeout        time.Duration
	json           bool
	quiet          bool
	plain          bool
	noColor        bool
}

func registerGlobals(fs *flag.FlagSet, g *globalFlags) {
	fs.StringVar(&g.server, "server", "", "server origin")
	fs.DurationVar(&g.timeout, "timeout", 12*time.Second, "request timeout")
	fs.BoolVar(&g.json, "json", false, "JSON output")
	fs.BoolVar(&g.quiet, "quiet", false, "less decoration")
	fs.BoolVar(&g.plain, "plain", false, "accessible line prompts")
	fs.BoolVar(&g.noColor, "no-color", false, "disable color")
}

func resolveConfig(env Env, g globalFlags) config {
	c := config{timeout: g.timeout, json: g.json, quiet: g.quiet, plain: g.plain, noColor: g.noColor, serverFlag: g.serverExplicit}
	switch {
	case g.serverExplicit:
		c.server = g.server
	case env.Getenv("BURNERPAD_SERVER") != "":
		c.server = env.Getenv("BURNERPAD_SERVER")
	default:
		c.server = defaultServer
	}
	if env.Getenv("BURNERPAD_PLAIN") == "1" || env.Getenv("CI") != "" || env.Getenv("TERM") == "dumb" {
		c.plain = true
	}
	if env.Getenv("NO_COLOR") != "" || env.Getenv("TERM") == "dumb" {
		c.noColor = true
	}
	return c
}

type clipFlag struct {
	enabled  bool
	duration time.Duration
}

func (c *clipFlag) String() string {
	if !c.enabled {
		return "false"
	}
	return c.duration.String()
}

func (c *clipFlag) Set(value string) error {
	if value == "true" || value == "" {
		c.enabled, c.duration = true, 45*time.Second
		return nil
	}
	if value == "false" {
		c.enabled = false
		return nil
	}
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 || d%time.Second != 0 {
		return fmt.Errorf("must be a positive whole-second duration")
	}
	c.enabled, c.duration = true, d
	return nil
}

func (*clipFlag) IsBoolFlag() bool { return true }

type createFlags struct {
	ttl, input, passphraseFile string
	passphraseFD               int
	ask, clip                  bool
}

func registerCreate(fs *flag.FlagSet) *createFlags {
	f := &createFlags{passphraseFD: -1}
	fs.StringVar(&f.ttl, "ttl", "", "time to live")
	fs.StringVar(&f.input, "input", "", "plaintext input file")
	fs.BoolVar(&f.ask, "ask", false, "prompt for an existing passphrase")
	fs.StringVar(&f.passphraseFile, "passphrase-file", "", "passphrase file")
	fs.IntVar(&f.passphraseFD, "passphrase-fd", -1, "passphrase file descriptor")
	fs.BoolVar(&f.clip, "clip", false, "copy the link using OSC 52")
	return f
}

type revealFlags struct {
	ask            bool
	passphraseFile string
	passphraseFD   int
	keepBlob, out  string
	clip           clipFlag
}

func registerReveal(fs *flag.FlagSet) *revealFlags {
	f := &revealFlags{passphraseFD: -1}
	fs.BoolVar(&f.ask, "ask", false, "prompt for the passphrase")
	fs.StringVar(&f.passphraseFile, "passphrase-file", "", "passphrase file")
	fs.IntVar(&f.passphraseFD, "passphrase-fd", -1, "passphrase file descriptor")
	fs.StringVar(&f.keepBlob, "keep-blob", "", "save claimed ciphertext")
	fs.StringVar(&f.out, "out", "", "plaintext output file")
	fs.Var(&f.clip, "clip", "copy plaintext using OSC 52")
	return f
}

type burnFlags struct {
	tokenFile string
	tokenFD   int
}

func registerBurn(fs *flag.FlagSet) *burnFlags {
	f := &burnFlags{tokenFD: -1}
	fs.StringVar(&f.tokenFile, "token-file", "", "management-token file")
	fs.IntVar(&f.tokenFD, "token-fd", -1, "management-token file descriptor")
	return f
}

type decryptFlags struct {
	blobFile       string
	ask            bool
	passphraseFile string
	passphraseFD   int
	out            string
	clip           clipFlag
}

func registerDecrypt(fs *flag.FlagSet) *decryptFlags {
	f := &decryptFlags{passphraseFD: -1}
	fs.StringVar(&f.blobFile, "blob-file", "", "canonical ciphertext file, or - for stdin")
	fs.BoolVar(&f.ask, "ask", false, "prompt for the passphrase")
	fs.StringVar(&f.passphraseFile, "passphrase-file", "", "passphrase file")
	fs.IntVar(&f.passphraseFD, "passphrase-fd", -1, "passphrase file descriptor")
	fs.StringVar(&f.out, "out", "", "plaintext output file")
	fs.Var(&f.clip, "clip", "copy plaintext using OSC 52")
	return f
}

func countSources(values ...bool) int {
	n := 0
	for _, value := range values {
		if value {
			n++
		}
	}
	return n
}

func trimOneNewline(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
		if len(b) > 0 && b[len(b)-1] == '\r' {
			b = b[:len(b)-1]
		}
	}
	return b
}

func oneLine(b []byte) string { return strings.TrimSpace(string(b)) }
