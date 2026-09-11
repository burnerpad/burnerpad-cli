//go:build linux || darwin || windows

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/burnerpad/burnerpad-cli/internal/secret"
)

func TestUnsafeCredentialFilesFailBeforeNetworkWithoutDisclosure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	for _, test := range []struct {
		name       string
		arguments  func(string) []string
		stdin      string
		stdinPiped bool
		wantServer bool
	}{
		{
			name: "create passphrase", stdin: "plaintext", stdinPiped: true, wantServer: true,
			arguments: func(path string) []string {
				return []string{"create", "--json", "--server", server.URL, "--passphrase-file", path}
			},
		},
		{
			name: "reveal passphrase", stdin: server.URL + "/s/" + contractID, stdinPiped: true, wantServer: true,
			arguments: func(path string) []string {
				return []string{"reveal", "--json", "--passphrase-file", path}
			},
		},
		{
			name: "burn token", wantServer: true,
			arguments: func(path string) []string {
				return []string{"burn", "--json", "--server", server.URL, "--token-file", path, contractID}
			},
		},
		{
			name: "decrypt passphrase",
			arguments: func(path string) []string {
				return []string{"decrypt", "--json", "--blob-file", filepath.Join(t.TempDir(), "missing"), "--passphrase-file", path}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := insecureCredentialFile(t, "private-path-canary", "private-content-canary")
			before := requests.Load()
			env, stdout, stderr := contractEnv(test.arguments(path), test.stdin)
			env.StdinPiped = test.stdinPiped
			if exit := Run(env); exit != 2 {
				t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
			}
			if requests.Load() != before {
				t.Fatalf("network request occurred before credential-file rejection")
			}
			var result map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("error JSON: %v; stdout=%q", err, stdout)
			}
			if result["code"] != "invalid_credential_source" {
				t.Fatalf("error code=%v", result["code"])
			}
			if test.wantServer && result["server"] != server.URL {
				t.Fatalf("server=%v; want %s", result["server"], server.URL)
			}
			combined := stdout.String() + stderr.String()
			for _, secret := range []string{path, "private-path-canary", "private-content-canary"} {
				if strings.Contains(combined, secret) {
					t.Fatalf("diagnostic disclosed %q: %q", secret, combined)
				}
			}
		})
	}
}

func TestMissingCredentialFilesRemainLocalIOFailures(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-path-canary")
	for _, arguments := range [][]string{
		{"create", "--json", "--server", "https://example.invalid", "--passphrase-file", missing},
		{"burn", "--json", "--server", "https://example.invalid", "--token-file", missing, contractID},
	} {
		env, stdout, stderr := contractEnv(arguments, "plaintext")
		env.StdinPiped = arguments[0] == "create"
		if exit := Run(env); exit != 3 {
			t.Fatalf("args=%v exit=%d stdout=%s stderr=%s", arguments, exit, stdout, stderr)
		}
		var result map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["code"] != "local_io_failed" || strings.Contains(stdout.String()+stderr.String(), missing) {
			t.Fatalf("args=%v result=%v stderr=%q", arguments, result, stderr)
		}
	}
}

func TestCredentialDescriptorsRemainPipeCapabilities(t *testing.T) {
	application := &application{}
	phrase, err := application.readPassphrase(false, "", pipeDescriptor(t, contractPhrase+"\n"))
	if err != nil {
		t.Fatalf("readPassphrase from pipe: %v", err)
	}
	defer secret.Wipe(phrase)
	if string(phrase) != contractPhrase {
		t.Fatalf("phrase=%q", phrase)
	}
	token, err := application.readToken(&burnFlags{tokenFD: pipeDescriptor(t, contractToken+"\n")})
	if err != nil {
		t.Fatalf("readToken from pipe: %v", err)
	}
	if token != contractToken {
		t.Fatalf("token=%q", token)
	}
}
