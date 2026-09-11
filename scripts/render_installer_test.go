//go:build !windows

package scripts

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerAcceptsOnlyDocumentedArguments(t *testing.T) {
	const (
		usage              = "usage: sh install.sh [--verify-only]\n"
		developmentRefusal = "this is the unreleased template installer; when a release is published,\n" +
			"obtain its rendered installer from\n" +
			"https://github.com/burnerpad/burnerpad-cli/releases\n"
	)
	tests := []struct {
		name       string
		arguments  []string
		exitCode   int
		wantStderr string
	}{
		{name: "install", exitCode: 1, wantStderr: developmentRefusal},
		{name: "verify only", arguments: []string{"--verify-only"}, exitCode: 1, wantStderr: developmentRefusal},
		{name: "misspelled", arguments: []string{"--verfy-only"}, exitCode: 2, wantStderr: usage},
		{name: "option delimiter", arguments: []string{"--"}, exitCode: 2, wantStderr: usage},
		{name: "help", arguments: []string{"-h"}, exitCode: 2, wantStderr: usage},
		{name: "empty argument", arguments: []string{""}, exitCode: 2, wantStderr: usage},
		{name: "assignment", arguments: []string{"--verify-only=true"}, exitCode: 2, wantStderr: usage},
		{name: "positional", arguments: []string{"unexpected"}, exitCode: 2, wantStderr: usage},
		{name: "duplicate", arguments: []string{"--verify-only", "--verify-only"}, exitCode: 2, wantStderr: usage},
		{name: "trailing", arguments: []string{"--verify-only", "unexpected"}, exitCode: 2, wantStderr: usage},
		{name: "leading", arguments: []string{"unexpected", "--verify-only"}, exitCode: 2, wantStderr: usage},
		{name: "many", arguments: []string{"one", "two", "three"}, exitCode: 2, wantStderr: usage},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			installDirectory := filepath.Join(t.TempDir(), "bin")
			command := exec.Command("sh", append([]string{"../install.sh"}, test.arguments...)...)
			command.Env = append(os.Environ(), "BURNERPAD_INSTALL_DIR="+installDirectory)
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) || exitError.ExitCode() != test.exitCode {
				t.Fatalf("exit error=%v; want code %d", err, test.exitCode)
			}
			if stdout.String() != "" {
				t.Errorf("stdout=%q; want empty", stdout.String())
			}
			if stderr.String() != test.wantStderr {
				t.Errorf("stderr=%q; want %q", stderr.String(), test.wantStderr)
			}
			if _, err := os.Stat(installDirectory); !os.IsNotExist(err) {
				t.Errorf("install directory exists or stat failed unexpectedly: %v", err)
			}
		})
	}
}

func TestRenderInstallerUsesTagAndReleaseChecksums(t *testing.T) {
	directory := t.TempDir()
	checksums := filepath.Join(directory, "SHA256SUMS")
	output := filepath.Join(directory, "install.sh")
	var lines []string
	for i, target := range []string{"linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64"} {
		lines = append(lines, strings.Repeat(string(rune('a'+i)), 64)+"  burnerpad_1.2.3_"+target+".tar.gz")
	}
	if err := os.WriteFile(checksums, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(
		"sh", "render-installer.sh", "../install.sh", checksums, output,
		"v1.2.3", "Cinderella-Man/burnerpad-cli",
	)
	if result, err := command.CombinedOutput(); err != nil {
		t.Fatalf("renderer failed: %v\n%s", err, result)
	}
	rendered, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(rendered)
	for _, want := range []string{
		`REPO="Cinderella-Man/burnerpad-cli"`,
		`VERSION="1.2.3"`,
		`SHA256_linux_amd64="` + strings.Repeat("a", 64) + `"`,
		`SHA256_linux_arm64="` + strings.Repeat("b", 64) + `"`,
		`SHA256_darwin_amd64="` + strings.Repeat("c", 64) + `"`,
		`SHA256_darwin_arm64="` + strings.Repeat("d", 64) + `"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered installer does not contain %q", want)
		}
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("rendered mode=%v", info.Mode().Perm())
	}
	command = exec.Command("sh", output, "--verfy-only")
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 2 {
		t.Fatalf("rendered installer accepted an unknown argument: %v", err)
	}
	if stdout.String() != "" || stderr.String() != "usage: sh install.sh [--verify-only]\n" {
		t.Fatalf("rendered installer diagnostic stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRenderInstallerRejectsInvalidTagAndIncompleteChecksums(t *testing.T) {
	directory := t.TempDir()
	checksums := filepath.Join(directory, "SHA256SUMS")
	if err := os.WriteFile(checksums, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{
		"1.2.3",
		"v1.2",
		"v01.2.3",
		"v1.02.3",
		"v1.2.03",
		"v1.2.3-01",
		"v1.2.3-",
		"v1.2.3-alpha..1",
		"v1.2.3/unsafe",
		"v1.2.3&unsafe",
		"v1.2.3\";touch-pwned;#",
		"v1.2.3$(touch-pwned)",
		"v1.2.3\nunsafe",
	} {
		command := exec.Command(
			"sh", "render-installer.sh", "../install.sh", checksums,
			filepath.Join(directory, "out"), tag, "Cinderella-Man/burnerpad-cli",
		)
		result, err := command.CombinedOutput()
		if err == nil {
			t.Errorf("tag %q was accepted", tag)
		} else if !strings.Contains(string(result), "invalid release tag") {
			t.Errorf("tag %q failed for the wrong reason: %s", tag, result)
		}
	}
	for _, repository := range []string{
		"",
		"owner",
		"owner/repo/extra",
		"owner/repo;touch-pwned",
		"owner/repo&unsafe",
		"owner/repo\nunsafe",
	} {
		command := exec.Command(
			"sh", "render-installer.sh", "../install.sh", checksums,
			filepath.Join(directory, "out"), "v1.2.3", repository,
		)
		result, err := command.CombinedOutput()
		if err == nil {
			t.Errorf("repository %q was accepted", repository)
		} else if repository != "" && !strings.Contains(string(result), "invalid release repository") {
			t.Errorf("repository %q failed for the wrong reason: %s", repository, result)
		}
	}

	command := exec.Command(
		"sh", "render-installer.sh", "../install.sh", checksums,
		filepath.Join(directory, "out"), "v1.2.3", "Cinderella-Man/burnerpad-cli",
	)
	if err := command.Run(); err == nil {
		t.Fatal("incomplete checksum manifest was accepted")
	}

	malicious := strings.Repeat("&", 64) + "  burnerpad_1.2.3_linux_amd64.tar.gz\n"
	if err := os.WriteFile(checksums, []byte(malicious), 0o600); err != nil {
		t.Fatal(err)
	}
	command = exec.Command(
		"sh", "render-installer.sh", "../install.sh", checksums,
		filepath.Join(directory, "out"), "v1.2.3", "Cinderella-Man/burnerpad-cli",
	)
	if err := command.Run(); err == nil {
		t.Fatal("non-hex checksum was accepted")
	}
}
