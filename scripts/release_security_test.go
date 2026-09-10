//go:build !windows

package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	permissionWriteLine = regexp.MustCompile(`(?m)^[ \t]+["']?[A-Za-z][A-Za-z0-9-]*["']?[ \t]*:[ \t]*write[ \t]*$`)
	inlinePermissions   = regexp.MustCompile(`(?m)^[ \t]*["']?permissions["']?[ \t]*:[ \t]*\S`)
	writeAllPermission  = regexp.MustCompile(`(?m)^[ \t]*["']?permissions["']?[ \t]*:[ \t]*write-all[ \t]*$`)
)

func TestOnlyPublisherWorkflowCanRequestWriteAuthority(t *testing.T) {
	paths, err := filepath.Glob("../.github/workflows/*.yml")
	if err != nil {
		t.Fatal(err)
	}
	yamlPaths, err := filepath.Glob("../.github/workflows/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, yamlPaths...)
	if len(paths) == 0 {
		t.Fatal("repository has no GitHub Actions workflows")
	}

	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		policy := stripYAMLComments(string(raw))
		if writeAllPermission.MatchString(policy) {
			t.Errorf("%s uses write-all", path)
		} else if inlinePermissions.MatchString(policy) {
			t.Errorf("%s uses scalar or inline permissions; use an auditable block", path)
		}
		writes := permissionWriteLine.FindAllString(policy, -1)
		if filepath.Base(path) != "publish-release.yml" && len(writes) != 0 {
			t.Errorf("non-publisher workflow %s requests write authority: %v", path, writes)
		}
	}
}

func TestReleaseWorkflowUsesTrustedDefaultBranchAndScopesAuthority(t *testing.T) {
	if _, err := os.Stat("../.github/workflows/release.yml"); !os.IsNotExist(err) {
		t.Fatalf("legacy tag-triggered workflow must stay removed; stat error: %v", err)
	}
	raw, err := os.ReadFile("../.github/workflows/publish-release.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := stripYAMLComments(string(raw))
	top, _, ok := strings.Cut(workflow, "jobs:\n")
	if !ok {
		t.Fatal("release workflow has no jobs block")
	}
	if !strings.Contains(top, "on:\n  repository_dispatch:\n    types: [release]\n") {
		t.Fatal("publisher is not triggered by trusted-default-branch repository_dispatch")
	}
	for _, forbidden := range []string{"push:", "workflow_dispatch:", "pull_request:", "release:"} {
		if strings.Contains(top, forbidden) {
			t.Errorf("publisher has unsafe/unexpected trigger %q", forbidden)
		}
	}
	if !strings.Contains(top, "permissions:\n  contents: read\n") || permissionWriteLine.MatchString(top) {
		t.Fatal("workflow-level permissions are not read-only")
	}
	if !strings.Contains(top, "concurrency:\n  group: publish-release\n  cancel-in-progress: false\n") {
		t.Fatal("publisher does not serialize release mutations without cancelling an active publication")
	}

	for name, job := range workflowJobs(t, workflow) {
		if writeAllPermission.MatchString(job) {
			t.Errorf("job %q uses write-all", name)
		} else if inlinePermissions.MatchString(job) {
			t.Errorf("job %q uses scalar or inline permissions; use an auditable block", name)
		}
		writes := permissionWriteLine.FindAllString(job, -1)
		if name != "publish-release" && len(writes) != 0 {
			t.Errorf("unprivileged job %q has write authority: %v", name, writes)
		}
		if name == "publish-release" {
			got := make(map[string]bool)
			for _, line := range writes {
				got[strings.TrimSpace(line)] = true
			}
			for _, want := range []string{"contents: write", "id-token: write"} {
				if !got[want] {
					t.Errorf("publisher lacks exact permission %q", want)
				}
			}
			if len(writes) != 2 {
				t.Errorf("publisher has unexpected write permissions: %v", writes)
			}
		}
	}

	validation := workflowJob(t, workflow, "validate-release-request")
	steps := workflowStepHeaders(validation)
	if len(steps) != 3 ||
		steps[0] != "- id: authorize" ||
		!strings.HasPrefix(steps[1], "- uses: actions/checkout@") ||
		steps[2] != "- name: Verify the checked-out default-branch tip" {
		t.Fatalf("authorization must be the first of exactly three validation steps; got %v", steps)
	}
	for _, want := range []string{
		`AUTHORIZED_ACTOR_ID: ${{ vars.RELEASE_ACTOR_ID }}`,
		`RELEASE_SHA: ${{ github.event.client_payload.sha }}`,
		`RELEASE_TAG: ${{ github.event.client_payload.tag }}`,
		`if [[ "$GITHUB_EVENT_NAME" != repository_dispatch ]]`,
		`if [[ -z "$AUTHORIZED_ACTOR_ID" ]] || [[ "$REQUEST_ACTOR_ID" != "$AUTHORIZED_ACTOR_ID" ]]`,
		`if [[ "$GITHUB_REF" != "$EXPECTED_REF" ]]`,
		`[[ ! "$RELEASE_SHA" =~ ^[0-9a-f]{40}$ ]]`,
		`[[ "$RELEASE_SHA" != "$GITHUB_SHA" ]]`,
		`if [[ ! "$RELEASE_TAG" =~ $release_tag_pattern ]]`,
		"persist-credentials: false",
		`test "$(git rev-parse --verify HEAD)" = "$RELEASE_SHA"`,
	} {
		if !strings.Contains(validation, want) {
			t.Errorf("release-request validation does not contain %q", want)
		}
	}

	for _, name := range []string{"gate", "pinned-lite-interop"} {
		job := workflowJob(t, workflow, name)
		if !strings.Contains(job, "needs: validate-release-request") {
			t.Errorf("job %q can run before release-request validation", name)
		}
	}
	publisher := workflowJob(t, workflow, "publish-release")
	for _, want := range []string{
		"needs: [validate-release-request, gate, pinned-lite-interop]",
		"GITHUB_TOKEN: ${{ github.token }}",
		"GH_TOKEN: ${{ github.token }}",
		"version: v2.18.1",
		"syft-version: v1.51.1",
		`GORELEASER_CURRENT_TAG: ${{ needs.validate-release-request.outputs.tag }}`,
		`"$RELEASE_TAG" "$GITHUB_REPOSITORY"`,
		`-f ref="refs/tags/$RELEASE_TAG" -f sha="$RELEASE_SHA"`,
		`test "$(gh release view "$RELEASE_TAG" --json isDraft --jq .isDraft)" = true`,
		`local_files=$(find dist -type f -printf '%f\n')`,
		`match_count=$(grep -Fxc -- "$release_asset" <<< "$local_files" || true)`,
		`[[ "$match_count" != 1 ]]`,
		`echo "draft release contains unexpected asset: $release_asset" >&2`,
		`SHA256SUMS SHA256SUMS.bundle install.sh`,
		`"burnerpad_${version}_linux_amd64.tar.gz"`,
		`"burnerpad_${version}_linux_arm64.tar.gz"`,
		`"burnerpad_${version}_darwin_amd64.tar.gz"`,
		`"burnerpad_${version}_darwin_arm64.tar.gz"`,
		`"burnerpad_${version}_windows_amd64.zip"`,
		`"burnerpad_${version}_windows_arm64.zip"`,
		`--certificate-identity "https://github.com/$GITHUB_WORKFLOW_REF"`,
		`--certificate-oidc-issuer https://token.actions.githubusercontent.com`,
		`gh release edit "$RELEASE_TAG" --draft=false`,
		`--json isDraft,isImmutable`,
		`--jq '.isDraft == false and .isImmutable == true'`,
		`for attempt in 1 2 3; do`,
		`[[ "$attempt" = 3 ]] || sleep $((attempt * 5))`,
		`echo "could not dispatch reproduction; use the documented manual fallback" >&2`,
		"Publish and lock the completed draft",
		"Request independent reproduction",
	} {
		if !strings.Contains(publisher, want) {
			t.Errorf("publisher does not contain %q", want)
		}
	}
	if strings.Contains(workflow, "RELEASE_TOKEN") {
		t.Error("publisher uses a long-lived release token")
	}
	if strings.Contains(publisher, "actions/attest") || strings.Contains(publisher, "attestations:") {
		t.Error("publisher unexpectedly adds a third provenance mechanism; update the documented trust model with it")
	}
	last := -1
	for _, marker := range []string{
		"Create or verify the exact release tag",
		"goreleaser/goreleaser-action@",
		"Render and attach the tag-derived installer",
		"Publish and lock the completed draft",
		"Request independent reproduction",
	} {
		index := strings.Index(publisher, marker)
		if index <= last {
			t.Errorf("publisher step %q is absent or out of order", marker)
		}
		last = index
	}
	publishIndex := strings.Index(publisher, `gh release edit "$RELEASE_TAG" --draft=false`)
	immutablePostconditionIndex := strings.Index(publisher, `--json isDraft,isImmutable`)
	if publishIndex < 0 || immutablePostconditionIndex <= publishIndex {
		t.Error("publisher does not verify immutability after publishing the draft")
	}

	for line, checkout := range checkoutBlocks(workflow) {
		if !strings.Contains(checkout, "persist-credentials: false") {
			t.Errorf("checkout at line %d persists credentials", line)
		}
	}
}

func TestInstallerRepositoryIsValidatedAndDerivedFromPublisher(t *testing.T) {
	raw, err := os.ReadFile("render-installer.sh")
	if err != nil {
		t.Fatal(err)
	}
	renderer := stripYAMLComments(string(raw))
	for _, want := range []string{
		`repository=${5:?release repository required}`,
		`*[!0-9A-Za-z._/-]*)`,
		`grep -Eq '^[0-9A-Za-z][0-9A-Za-z-]*/[0-9A-Za-z._-]+$'`,
		`-e "s|^REPO=.*|REPO=\"$repository\"|"`,
	} {
		if !strings.Contains(renderer, want) {
			t.Errorf("installer renderer does not contain repository safeguard %q", want)
		}
	}
}

func TestReleaseDocumentationRequiresImmutableReleases(t *testing.T) {
	raw, err := os.ReadFile("../RELEASING.md")
	if err != nil {
		t.Fatal(err)
	}
	documentation := string(raw)
	for _, want := range []string{
		"Enable immutable releases and verify that",
		"gh api repos/OWNER/REPOSITORY/immutable-releases --jq .enabled",
		"Set `RELEASE_ACTOR_ID` only after the preceding controls are verified",
		"The publisher retries that dispatch three times.",
		"gh workflow run repro-verify.yml",
		"-f tag=vX.Y.Z",
	} {
		if !strings.Contains(documentation, want) {
			t.Errorf("release documentation does not contain prerequisite %q", want)
		}
	}
}

func TestReleaseReproductionMatchesPublisherMetadataAndAuthenticatesAssets(t *testing.T) {
	raw, err := os.ReadFile("../.github/workflows/repro-verify.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := stripYAMLComments(string(raw))
	for _, want := range []string{
		`C=$(git rev-parse --verify HEAD)`,
		`D=$(TZ=UTC git show -s --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ HEAD)`,
		`API_TOKEN: ${{ github.token }}`,
		`-H "Authorization: Bearer $API_TOKEN"`,
		`-H "Accept: application/octet-stream"`,
		`"https://api.github.com/repos/$REPOSITORY/releases/assets/$asset_id"`,
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("reproduction workflow does not contain %q", want)
		}
	}
	if strings.Count(workflow, `-H "Authorization: Bearer $API_TOKEN"`) < 2 {
		t.Error("reproduction workflow does not authenticate both release lookup and asset download")
	}
	for _, forbidden := range []string{
		"git rev-parse --short",
		"--date=format:%Y-%m-%d",
		"https://github.com/${{ github.repository }}/releases/download/",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("reproduction workflow contains stale or unauthenticated build logic %q", forbidden)
		}
	}
}

func TestGoReleaserUsesCanonicalMetadataAndRecoverableDraft(t *testing.T) {
	raw, err := os.ReadFile("../.goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	config := stripYAMLComments(string(raw))
	for _, want := range []string{
		`-X main.version={{ .Version }} -X main.commit={{ .FullCommit }} -X main.date={{ .CommitDate }}`,
		"release:\n  draft: true\n  replace_existing_draft: true\n  prerelease: auto\n  preflight:\n    fail_on_error: true",
	} {
		if !strings.Contains(config, want) {
			t.Errorf("GoReleaser configuration does not contain %q", want)
		}
	}
	if strings.Contains(config, "main.commit={{ .Commit }}") {
		t.Error("GoReleaser uses an ambiguous/non-full commit template")
	}
}

func TestReleaseWorkflowUsesCanonicalSemVer(t *testing.T) {
	paths := []string{
		"../.github/workflows/publish-release.yml",
		"../.github/workflows/repro-verify.yml",
		"render-installer.sh",
	}
	var sourcePattern string
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		match := regexp.MustCompile(`(?m)^\s*release_tag_pattern='([^']+)'$`).FindStringSubmatch(string(raw))
		if match == nil {
			t.Fatalf("%s has no extractable release tag pattern", path)
		}
		if !strings.Contains(string(raw), "export LC_ALL=C") {
			t.Errorf("%s does not pin ASCII character semantics", path)
		}
		if sourcePattern == "" {
			sourcePattern = match[1]
		} else if match[1] != sourcePattern {
			t.Errorf("%s release tag policy drifted from the publisher", path)
		}
	}
	pattern, err := regexp.Compile(sourcePattern)
	if err != nil {
		t.Fatalf("release tag pattern does not compile: %v", err)
	}
	for _, tag := range []string{"v0.0.0", "v1.2.3", "v1.2.3-rc.1", "v1.2.3-0.3.7", "v1.2.3+build.5", "v1.2.3-rc.1+build.5"} {
		if !pattern.MatchString(tag) {
			t.Errorf("canonical tag %q was rejected", tag)
		}
	}
	for _, tag := range []string{
		"1.2.3",
		"v01.2.3",
		"v1.02.3",
		"v1.2.03",
		"v1.2",
		"v1.2.3-01",
		"v1.2.3-alpha..1",
		"v1.2.3/unsafe",
		"v1.2.3\";touch-pwned;#",
		"v1.2.3$(touch-pwned)",
		"v1.2.3\nunsafe",
	} {
		if pattern.MatchString(tag) {
			t.Errorf("unsafe/noncanonical tag %q was accepted", tag)
		}
	}
}

func TestMakeRejectsShellSyntaxInBuildMetadata(t *testing.T) {
	for _, target := range []string{"build", "cross"} {
		t.Run(target, func(t *testing.T) {
			directory := t.TempDir()
			marker := filepath.Join(directory, "pwned")
			goCalled := filepath.Join(directory, "go-called")
			fakeBin := filepath.Join(directory, "bin")
			if err := os.Mkdir(fakeBin, 0o700); err != nil {
				t.Fatal(err)
			}
			fakeGo := filepath.Join(fakeBin, "go")
			if err := os.WriteFile(fakeGo, []byte("#!/bin/sh\n: > \"$BURNERPAD_TEST_GO_CALLED\"\n"), 0o700); err != nil {
				t.Fatal(err)
			}

			version := `v1.2.3";>"$BURNERPAD_TEST_MARKER";#`
			if result, err := exec.Command("git", "check-ref-format", "refs/tags/"+version).CombinedOutput(); err != nil {
				t.Fatalf("attack fixture is not a legal Git tag: %v\n%s", err, result)
			}
			command := exec.Command("make", target)
			command.Dir = ".."
			command.Env = environmentWith(map[string]string{
				"PATH":                     fakeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
				"VERSION":                  version,
				"COMMIT":                   "e8edfba",
				"DATE":                     "2026-09-10",
				"BURNERPAD_TEST_GO_CALLED": goCalled,
				"BURNERPAD_TEST_MARKER":    marker,
			})
			result, err := command.CombinedOutput()
			if err == nil {
				t.Fatalf("unsafe build metadata was accepted: %s", result)
			}
			if !strings.Contains(string(result), "invalid VERSION build metadata") {
				t.Fatalf("unsafe metadata failed for the wrong reason: %s", result)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("build metadata executed shell syntax; marker stat error: %v", err)
			}
			if _, err := os.Stat(goCalled); !os.IsNotExist(err) {
				t.Fatalf("go ran despite rejected metadata; marker stat error: %v", err)
			}
			if strings.Contains(string(result), marker) {
				t.Fatal("rejected metadata was echoed to the build log")
			}
		})
	}
}

func TestMakeDoesNotExpandCommandLineMetadata(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, "make-expanded")
	goCalled := filepath.Join(directory, "go-called")
	fakeBin := filepath.Join(directory, "bin")
	if err := os.Mkdir(fakeBin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fakeBin, "go"), []byte("#!/bin/sh\n: > \"$BURNERPAD_TEST_GO_CALLED\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(
		"make",
		"build",
		"VERSION=$(shell >"+marker+")",
		"COMMIT=e8edfba",
		"DATE=2026-09-10",
	)
	command.Dir = ".."
	command.Env = environmentWith(map[string]string{
		"PATH":                     fakeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"BURNERPAD_TEST_GO_CALLED": goCalled,
	})
	result, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(result), "invalid VERSION build metadata") {
		t.Fatalf("unsafe command-line metadata was not rejected cleanly: %v\n%s", err, result)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("make expanded command-line metadata; marker stat error: %v", err)
	}
	if _, err := os.Stat(goCalled); !os.IsNotExist(err) {
		t.Fatalf("go ran despite rejected metadata; marker stat error: %v", err)
	}
}

func TestMakePassesValidMetadataAsOneLinkerArgument(t *testing.T) {
	directory := t.TempDir()
	argsPath := filepath.Join(directory, "go-args")
	fakeBin := filepath.Join(directory, "bin")
	if err := os.Mkdir(fakeBin, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeGo := filepath.Join(fakeBin, "go")
	fake := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$BURNERPAD_TEST_GO_ARGS\"\n"
	if err := os.WriteFile(fakeGo, []byte(fake), 0o700); err != nil {
		t.Fatal(err)
	}

	command := exec.Command("make")
	command.Dir = ".."
	command.Env = environmentWith(map[string]string{
		"PATH":                   fakeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"VERSION":                "v1.2.3-rc.1+build.5",
		"COMMIT":                 "e8edfba",
		"DATE":                   "2026-09-10",
		"BURNERPAD_TEST_GO_ARGS": argsPath,
	})
	if result, err := command.CombinedOutput(); err != nil {
		t.Fatalf("valid build metadata failed: %v\n%s", err, result)
	}
	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "-s -w -buildid= -X main.version=v1.2.3-rc.1+build.5 -X main.commit=e8edfba -X main.date=2026-09-10"
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	found := false
	for i, line := range lines {
		if line == "-ldflags" && i+1 < len(lines) && lines[i+1] == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("linker metadata was not one exact argument; args:\n%s", raw)
	}
}

func workflowJobs(t *testing.T, workflow string) map[string]string {
	t.Helper()
	_, body, ok := strings.Cut(workflow, "jobs:\n")
	if !ok {
		t.Fatal("release workflow has no jobs block")
	}
	lines := strings.Split(body, "\n")
	jobs := make(map[string]string)
	for i, line := range lines {
		if !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "    ") || !strings.HasSuffix(line, ":") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimSpace(line), ":")
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(lines[j], "  ") && !strings.HasPrefix(lines[j], "    ") && strings.HasSuffix(lines[j], ":") {
				end = j
				break
			}
		}
		jobs[name] = strings.Join(lines[i:end], "\n")
	}
	if len(jobs) == 0 {
		t.Fatal("release workflow has no jobs")
	}
	return jobs
}

func workflowStepHeaders(job string) []string {
	var steps []string
	for _, line := range strings.Split(job, "\n") {
		if strings.HasPrefix(line, "      - ") {
			steps = append(steps, strings.TrimSpace(line))
		}
	}
	return steps
}

func stripYAMLComments(value string) string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if index := strings.Index(line, " #"); index >= 0 {
			line = line[:index]
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func workflowJob(t *testing.T, workflow, name string) string {
	t.Helper()
	lines := strings.Split(workflow, "\n")
	header := "  " + name + ":"
	start := -1
	for i, line := range lines {
		if line == header {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("release workflow has no %q job", name)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(line, ":") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

func checkoutBlocks(workflow string) map[int]string {
	lines := strings.Split(workflow, "\n")
	blocks := make(map[int]string)
	for i, line := range lines {
		if !strings.Contains(line, "uses: actions/checkout@") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			trimmed := strings.TrimSpace(lines[j])
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			currentIndent := len(lines[j]) - len(strings.TrimLeft(lines[j], " "))
			if currentIndent <= indent {
				end = j
				break
			}
		}
		blocks[i+1] = strings.Join(lines[i:end], "\n")
	}
	return blocks
}

func environmentWith(replacements map[string]string) []string {
	environment := make([]string, 0, len(os.Environ())+len(replacements))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := replacements[key]; !replaced {
			environment = append(environment, entry)
		}
	}
	for key, value := range replacements {
		environment = append(environment, key+"="+value)
	}
	return environment
}
