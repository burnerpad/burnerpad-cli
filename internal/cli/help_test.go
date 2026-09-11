package cli

import (
	"io"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/burnerpad/burnerpad-cli/internal/term"
)

type helpProbeReader struct{ reads *atomic.Int32 }

func (r helpProbeReader) Read([]byte) (int, error) {
	r.reads.Add(1)
	return 0, io.EOF
}

func TestHelpListsEveryCommandAndDirectForm(t *testing.T) {
	wantCommands := []string{"create", "reveal", "burn", "decrypt", "words", "completion", "version", "licenses", "help"}
	if got := completionCommandNames(buildCompletionSchema().commands); !slices.Equal(got, wantCommands) {
		t.Fatalf("commands = %v, want %v", got, wantCommands)
	}
	e, stdout, stderr := contractEnv([]string{"help"}, "")
	if code := Run(e); code != 0 {
		t.Fatalf("help exit=%d stderr=%s", code, stderr)
	}
	text := stdout.String()
	for _, spec := range commandSpecs {
		if !strings.Contains(text, "  "+spec.name) || !strings.Contains(text, spec.description) {
			t.Errorf("general help omits %s: %s", spec.name, text)
		}
	}
	for _, want := range []string{
		"burnerpad --help | -h", "burnerpad --version", "burnerpad help COMMAND", "burnerpad COMMAND --help", "burnerpad COMMAND -h",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("general help omits %q", want)
		}
	}
	for _, alias := range []string{"--help", "-h"} {
		e, aliasOut, aliasErr := contractEnv([]string{alias}, "")
		if code := Run(e); code != 0 || aliasErr.Len() != 0 {
			t.Fatalf("root %s exit=%d stderr=%s", alias, code, aliasErr)
		}
		if aliasOut.String() != text {
			t.Fatalf("root %s differs from help command:\n%s\nwant:\n%s", alias, aliasOut, text)
		}
	}
}

func TestEveryCommandHelpTopicAndAliasAreIdentical(t *testing.T) {
	for i := range commandSpecs {
		spec := &commandSpecs[i]
		t.Run(spec.name, func(t *testing.T) {
			referenceEnv, referenceOut, referenceErr := contractEnv([]string{"help", spec.name}, "")
			if code := Run(referenceEnv); code != 0 {
				t.Fatalf("help %s exit=%d stderr=%s", spec.name, code, referenceErr)
			}
			if !strings.Contains(referenceOut.String(), "Usage:\n  burnerpad "+spec.name) ||
				!strings.Contains(referenceOut.String(), spec.details) {
				t.Fatalf("help %s is incomplete:\n%s", spec.name, referenceOut)
			}
			for _, option := range optionsForCommand(spec) {
				label := option.name
				if option.takesValue {
					if option.valueName == "" {
						t.Fatalf("%s option %s has no help value name", spec.name, option.name)
					}
					label += " " + option.valueName
				}
				if !strings.Contains(referenceOut.String(), label) {
					t.Errorf("help %s omits %s:\n%s", spec.name, label, referenceOut)
				}
			}

			for _, alias := range []string{"--help", "-h"} {
				var reads, ttyOpens atomic.Int32
				e, stdout, stderr := contractEnv([]string{spec.name, alias}, "")
				e.Stdin = helpProbeReader{reads: &reads}
				e.StdinPiped = true
				e.OpenTTY = func() (*term.TTY, error) {
					ttyOpens.Add(1)
					return nil, io.ErrClosedPipe
				}
				if code := Run(e); code != 0 {
					t.Fatalf("%s %s exit=%d stderr=%s", spec.name, alias, code, stderr)
				}
				if stdout.String() != referenceOut.String() {
					t.Fatalf("%s %s differs from help topic:\n%s\nwant:\n%s", spec.name, alias, stdout, referenceOut)
				}
				if stderr.Len() != 0 || reads.Load() != 0 || ttyOpens.Load() != 0 {
					t.Fatalf("%s %s performed work: stderr=%q reads=%d tty_opens=%d",
						spec.name, alias, stderr, reads.Load(), ttyOpens.Load())
				}
			}
		})
	}
}

func TestRevealHelpShowsOptionalArgvURL(t *testing.T) {
	e, stdout, stderr := contractEnv([]string{"help", "reveal"}, "")
	if code := Run(e); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout.String(), "[FULL_SHARE_URL]") {
		t.Fatalf("reveal help makes its argv URL look mandatory:\n%s", stdout)
	}
}
