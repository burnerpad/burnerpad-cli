package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCompletionSchemaHasExactAcceptedOptions(t *testing.T) {
	schema := buildCompletionSchema()
	wantCommands := []string{"create", "reveal", "burn", "decrypt", "words", "completion", "version", "licenses", "help"}
	if got := completionCommandNames(schema.commands); !slices.Equal(got, wantCommands) {
		t.Fatalf("commands = %v, want %v", got, wantCommands)
	}
	if got, want := completionNames(schema.leading), []string{"--json", "--no-color", "--plain", "--server", "--timeout"}; !slices.Equal(got, want) {
		t.Fatalf("leading options = %v, want %v", got, want)
	}

	wantOptions := map[string][]string{
		"create":     {"--ask", "--input", "--json", "--no-color", "--passphrase-fd", "--passphrase-file", "--plain", "--server", "--timeout", "--ttl"},
		"reveal":     {"--ask", "--json", "--keep-blob", "--no-color", "--out", "--passphrase-fd", "--passphrase-file", "--plain", "--server", "--timeout"},
		"burn":       {"--json", "--server", "--timeout", "--token-fd", "--token-file"},
		"decrypt":    {"--ask", "--blob-file", "--json", "--no-color", "--out", "--passphrase-fd", "--passphrase-file", "--plain"},
		"words":      {},
		"completion": {},
		"version":    {},
		"licenses":   {},
		"help":       {},
	}
	for _, command := range schema.commands {
		if got, want := completionNames(command.flags), wantOptions[command.name]; !slices.Equal(got, want) {
			t.Errorf("%s options = %v, want %v", command.name, got, want)
		}
	}
}

func TestCompletionSchemaDistinguishesFileAndDescriptorValues(t *testing.T) {
	for _, command := range buildCompletionSchema().commands {
		for _, option := range command.flags {
			switch {
			case strings.HasSuffix(option.name, "-file") || option.name == "--input" || option.name == "--out" || option.name == "--keep-blob":
				if !option.fileValue {
					t.Errorf("%s %s is not marked as a file value", command.name, option.name)
				}
			case strings.HasSuffix(option.name, "-fd") && option.fileValue:
				t.Errorf("%s %s incorrectly enables file completion", command.name, option.name)
			}
		}
	}
}

func runBashCompletion(t *testing.T, words ...string) []string {
	return runBashCompletionFrom(t, "", words...)
}

func runBashCompletionFrom(t *testing.T, directory string, words ...string) []string {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is unavailable")
	}
	script, ok := completionScript("bash")
	if !ok {
		t.Fatal("bash completion is unavailable")
	}
	var input strings.Builder
	input.WriteString(script)
	input.WriteString("\nCOMP_WORDS=(")
	for _, word := range words {
		fmt.Fprintf(&input, " %q", word)
	}
	fmt.Fprintf(&input, " )\nCOMP_CWORD=%d\n_burnerpad\nprintf '%%s\\0' \"${COMPREPLY[@]}\"\n", len(words)-1)
	cmd := exec.Command(bash, "--noprofile", "--norc")
	cmd.Dir = directory
	cmd.Stdin = strings.NewReader(input.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash completion failed: %v\n%s", err, out)
	}
	text := strings.TrimSuffix(string(out), "\x00")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\x00")
}

func TestBashCompletionResolvesCommandAfterLeadingOptions(t *testing.T) {
	for _, test := range []struct {
		name  string
		words []string
		want  []string
	}{
		{name: "boolean before command", words: []string{"burnerpad", "--plain", "reveal", "--k"}, want: []string{"--keep-blob"}},
		{name: "valued before command", words: []string{"burnerpad", "--server", "https://example.test", "create", "--t"}, want: []string{"--timeout", "--ttl"}},
		{name: "command-looking value", words: []string{"burnerpad", "--server", "create", "reveal", "--k"}, want: []string{"--keep-blob"}},
		{name: "assigned value", words: []string{"burnerpad", "--server=create", "reveal", "--k"}, want: []string{"--keep-blob"}},
		{name: "offline option", words: []string{"burnerpad", "decrypt", "--b"}, want: []string{"--blob-file"}},
		{name: "unsupported offline timeout", words: []string{"burnerpad", "decrypt", "--t"}, want: nil},
		{name: "leading option value", words: []string{"burnerpad", "--server", "--burnerpad-no-value-match"}, want: nil},
		{name: "scalar option value", words: []string{"burnerpad", "create", "--ttl", "--burnerpad-no-value-match"}, want: nil},
		{name: "file option value", words: []string{"burnerpad", "create", "--input", "--burnerpad-no-file-match"}, want: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := runBashCompletion(t, test.words...); !slices.Equal(got, test.want) {
				t.Fatalf("completions = %v, want %v", got, test.want)
			}
		})
	}
}

func TestBashCompletionUsesNativeFileCandidates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload secret.txt")
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A Git Bash user supplies a shell-native path, not filepath.Join's Win32
	// backslash spelling. A relative prefix exercises that contract everywhere.
	got := runBashCompletionFrom(t, dir, "burnerpad", "create", "--input", "pay")
	requireFileCompletion(t, got, dir, path)
}

func runZshCompletion(t *testing.T, words ...string) []string {
	t.Helper()
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is unavailable")
	}
	script, _ := completionScript("zsh")
	var input strings.Builder
	input.WriteString("compdef() { :; }\n_describe() { print -rl -- \"${(@P)2}\"; }\n_files() { print -r -- __FILE_COMPLETION__; }\n")
	input.WriteString(script)
	input.WriteString("\nwords=(")
	for _, word := range words {
		input.WriteByte(' ')
		input.WriteString(zshQuote(word))
	}
	fmt.Fprintf(&input, " )\nCURRENT=%d\n_burnerpad\n", len(words))
	cmd := exec.Command(zsh, "-f")
	cmd.Stdin = strings.NewReader(input.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zsh completion failed: %v\n%s", err, out)
	}
	return strings.Fields(string(out))
}

func TestZshCompletionSkipsCommandLookingOptionValues(t *testing.T) {
	got := runZshCompletion(t, "burnerpad", "--server", "create", "reveal", "--k")
	if !slices.Contains(got, "--keep-blob") || slices.Contains(got, "--ttl") {
		t.Fatalf("reveal completions after command-looking value = %v", got)
	}
}

func TestZshCompletionRespectsOptionValuePositions(t *testing.T) {
	if got := runZshCompletion(t, "burnerpad", "--server", "--burnerpad-no-value-match"); len(got) != 0 {
		t.Fatalf("leading server value completions = %v, want none", got)
	}
	if got := runZshCompletion(t, "burnerpad", "create", "--ttl", "--burnerpad-no-value-match"); len(got) != 0 {
		t.Fatalf("TTL value completions = %v, want none", got)
	}
	if got := runZshCompletion(t, "burnerpad", "create", "--input", "--burnerpad-no-file-match"); !slices.Equal(got, []string{"__FILE_COMPLETION__"}) {
		t.Fatalf("input value completions = %v, want native file completion", got)
	}
}

func TestFishCompletionUsesResolvedCommand(t *testing.T) {
	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish is unavailable")
	}
	script, _ := completionScript("fish")
	input := script + "\ncomplete -C " + fishQuote("burnerpad --server create reveal --t") + "\n"
	cmd := exec.Command(fish)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fish completion failed: %v\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "--timeout") || strings.Contains(text, "--ttl") || strings.Contains(text, "--input") {
		t.Fatalf("reveal completions after command-looking value = %q", text)
	}
}

func TestFishCompletionRespectsOptionValuePositions(t *testing.T) {
	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish is unavailable")
	}
	script, _ := completionScript("fish")
	input := script + "\ncomplete -C " + fishQuote("burnerpad create --input --burnerpad-no-file-match") + "\n"
	cmd := exec.Command(fish)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fish completion failed: %v\n%s", err, out)
	}
	if text := string(out); strings.Contains(text, "--ttl") || strings.Contains(text, "--timeout") {
		t.Fatalf("input value offered options: %q", text)
	}
}

type powerShellCompletionResult struct {
	text    string
	toolTip string
}

func runPowerShellCompletion(t *testing.T, line, word string) []powerShellCompletionResult {
	t.Helper()
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("PowerShell is unavailable")
	}
	script, _ := completionScript("powershell")
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	var input strings.Builder
	input.WriteString("function Register-ArgumentCompleter { param([switch]$Native, [string]$CommandName, [scriptblock]$ScriptBlock) $global:BurnerpadCompleter = $ScriptBlock }\n")
	input.WriteString(script)
	input.WriteString("\n$tokens = $null\n$parseErrors = $null\n")
	fmt.Fprintf(&input, "$line = %s\n", quote(line))
	input.WriteString("$ast = [System.Management.Automation.Language.Parser]::ParseInput($line, [ref]$tokens, [ref]$parseErrors)\n")
	input.WriteString("$commandAst = $ast.EndBlock.Statements[0].PipelineElements[0]\n")
	fmt.Fprintf(&input, "$results = @(& $global:BurnerpadCompleter %s $commandAst $line.Length)\n", quote(word))
	input.WriteString("$results | ForEach-Object { [Console]::Out.Write([string]$_.CompletionText); [Console]::Out.Write([char]0); [Console]::Out.Write([string]$_.ToolTip); [Console]::Out.Write([char]0) }\n")
	cmd := exec.Command(pwsh, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", input.String())
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("PowerShell completion failed: %v\n%s", err, stderr.String())
	}
	if len(out) == 0 {
		return nil
	}
	if out[len(out)-1] != 0 {
		t.Fatalf("PowerShell completion output is not NUL-terminated: %q", out)
	}
	fields := strings.Split(string(out[:len(out)-1]), "\x00")
	if len(fields)%2 != 0 {
		t.Fatalf("PowerShell completion output has an incomplete record: %q", out)
	}
	results := make([]powerShellCompletionResult, 0, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		results = append(results, powerShellCompletionResult{text: fields[i], toolTip: fields[i+1]})
	}
	return results
}

func powerShellCompletionTexts(results []powerShellCompletionResult) []string {
	texts := make([]string, len(results))
	for i, result := range results {
		texts[i] = result.text
	}
	return texts
}

func powerShellCompletionToolTips(results []powerShellCompletionResult) []string {
	toolTips := make([]string, len(results))
	for i, result := range results {
		toolTips[i] = result.toolTip
	}
	return toolTips
}

func requireFileCompletion(t *testing.T, candidates []string, directory, want string) {
	t.Helper()
	if len(candidates) != 1 {
		t.Fatalf("file completions = %v, want exactly one candidate for %s", candidates, want)
	}
	// Windows shells may expand the RUNNER~1 form used by os.TempDir to its
	// long spelling. Compare the underlying file rather than path text.
	wantInfo, err := os.Stat(want)
	if err != nil {
		t.Fatal(err)
	}
	candidate := candidates[0]
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(directory, candidate)
	}
	candidateInfo, err := os.Stat(candidate)
	if err != nil {
		t.Fatalf("stat completion %q: %v", candidate, err)
	}
	if !os.SameFile(candidateInfo, wantInfo) {
		t.Fatalf("file completion %q does not identify %s", candidate, want)
	}
}

func TestPowerShellCompletionRespectsOptionValuePositions(t *testing.T) {
	if got := runPowerShellCompletion(t, "burnerpad --server ", ""); len(got) != 0 {
		t.Fatalf("leading server value completions = %v, want none", got)
	}
	got := runPowerShellCompletion(t, "burnerpad create --input --burnerpad-no-file-match", "--burnerpad-no-file-match")
	texts := powerShellCompletionTexts(got)
	if slices.Contains(texts, "--ttl") || slices.Contains(texts, "--timeout") {
		t.Fatalf("input value offered options: %v", got)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "payload secret.txt")
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Join(dir, "pay")
	got = runPowerShellCompletion(t, "burnerpad create --input "+prefix, prefix)
	requireFileCompletion(t, powerShellCompletionToolTips(got), "", path)
}

func TestPowerShellCompletionParses(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("PowerShell is unavailable")
	}
	script, _ := completionScript("powershell")
	cmd := exec.Command(pwsh, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", "-")
	cmd.Stdin = strings.NewReader(script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell completion failed to parse: %v\n%s", err, out)
	}
}
