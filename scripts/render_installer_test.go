//go:build !windows

package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
