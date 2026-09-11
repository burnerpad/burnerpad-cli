package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/burnerpad/burnerpad-cli/envelope"
	"github.com/burnerpad/burnerpad-cli/wordlist"
)

type jsonErrorRunner func(*testing.T) (exit int, stdout, server string)

var expectedMachineErrorExits = map[string]int{
	"invalid_command":           2,
	"invalid_option":            2,
	"invalid_input":             2,
	"invalid_credential_source": 2,
	"local_io_failed":           3,
	"secret_unavailable":        4,
	"passphrase_failed":         5,
	"plaintext_invalid":         5,
	"server_rejected":           6,
	"network_unavailable":       7,
	"rate_limited":              7,
	"service_unavailable":       7,
	"unsupported_secret":        8,
	"create_outcome_unknown":    9,
	"claim_outcome_unknown":     9,
	"revoke_outcome_unknown":    9,
	"internal":                  10,
}

func TestMachineErrorRegistryMatchesVersionOneContract(t *testing.T) {
	if len(machineErrorExits) != len(expectedMachineErrorExits) {
		t.Errorf("machineErrorExits has %d entries, want %d", len(machineErrorExits), len(expectedMachineErrorExits))
	}
	for code, exit := range expectedMachineErrorExits {
		if got, ok := machineErrorExits[code]; !ok || got != exit {
			t.Errorf("machineErrorExits[%q]=%d, %v; want %d, true", code, got, ok, exit)
		}
	}
	for code, exit := range machineErrorExits {
		if got, ok := expectedMachineErrorExits[code]; !ok || got != exit {
			t.Errorf("unexpected machineErrorExits mapping %q=%d", code, exit)
		}
	}
}

func TestReportRejectsUnregisteredOrMismatchedMachineErrors(t *testing.T) {
	retryAfter := int64(17)
	for _, command := range []commandError{
		{exit: 2, code: "new_unreviewed_code", message: "unreviewed detail", server: "https://secret.example", retryAfter: &retryAfter},
		{exit: 3, code: "invalid_input", message: "mismatched detail", server: "https://secret.example", retryAfter: &retryAfter},
	} {
		var stdout bytes.Buffer
		a := application{env: Env{Stdout: &stdout, Stderr: io.Discard}, cfg: config{json: true}}
		if exit := a.report(command); exit != 10 {
			t.Errorf("report(%q/%d) exit=%d, want 10", command.code, command.exit, exit)
		}
		const want = "{\"status\":\"error\",\"code\":\"internal\",\"message\":\"unexpected internal failure\"}\n"
		if stdout.String() != want {
			t.Errorf("report(%q/%d)=%q, want generic internal result %q", command.code, command.exit, stdout.String(), want)
		}
	}
}

func TestCurrentJSONErrorVocabularyAndExitMapping(t *testing.T) {
	responseServer := func(status int, body string, retryAfter ...int64) jsonErrorRunner {
		return func(t *testing.T) (int, string, string) {
			t.Helper()
			srv, _ := contractResponseServer(t, status, body, retryAfter...)
			token := credentialFile(t, "token", contractToken+"\n")
			env, stdout, _ := contractEnv([]string{
				"burn", "--server", srv.URL, "--json", "--token-file", token, contractID,
			}, "")
			return Run(env), stdout.String(), srv.URL
		}
	}
	createResponse := func(status int, body string) jsonErrorRunner {
		return func(t *testing.T) (int, string, string) {
			t.Helper()
			srv, _ := contractResponseServer(t, status, body)
			env, stdout, _ := contractEnv([]string{
				"create", "--server", srv.URL, "--json", "--passphrase-file", phraseFile(t),
			}, "plaintext")
			env.StdinPiped = true
			return Run(env), stdout.String(), srv.URL
		}
	}

	retryAfter := int64(17)
	tests := []struct {
		name, code, message string
		exit                int
		retryAfter          *int64
		run                 jsonErrorRunner
	}{
		{
			name: "invalid command", exit: 2, code: "invalid_command",
			message: "a command is required; run 'burnerpad help'",
			run:     commandErrorRun([]string{"--json"}, "", nil),
		},
		{
			name: "invalid option", exit: 2, code: "invalid_option",
			message: "invalid or unsupported option",
			run:     commandErrorRun([]string{"create", "--json", "--not-an-option"}, "", nil),
		},
		{
			name: "invalid input", exit: 2, code: "invalid_input",
			message: "create does not accept positional arguments",
			run:     commandErrorRun([]string{"create", "--json", "unexpected"}, "", nil),
		},
		{
			name: "invalid credential source", exit: 2, code: "invalid_credential_source",
			message: "passphrase files must be named and passphrase descriptors must be 3 or greater",
			run: commandErrorRun([]string{"create", "--json", "--passphrase-file", "-"}, "plaintext", func(env *Env) {
				env.StdinPiped = true
			}),
		},
		{
			name: "local I/O failure", exit: 3, code: "local_io_failed",
			message: "cannot read the ciphertext file",
			run: func(t *testing.T) (int, string, string) {
				t.Helper()
				missing := filepath.Join(t.TempDir(), "missing")
				return commandErrorRun([]string{
					"decrypt", "--json", "--blob-file", missing, "--passphrase-file", phraseFile(t),
				}, "", nil)(t)
			},
		},
		{
			name: "secret unavailable", exit: 4, code: "secret_unavailable",
			message: "the secret is unavailable",
			run:     responseServer(http.StatusNotFound, `{}`),
		},
		{
			name: "passphrase failed", exit: 5, code: "passphrase_failed",
			message: "the passphrase did not open the secret",
			run: decryptErrorRun(func() ([]byte, string) {
				return []byte("plaintext"), "freeway faucet unnoticed energy emoticon elves enormous"
			}),
		},
		{
			name: "plaintext invalid", exit: 5, code: "plaintext_invalid",
			message: "the authenticated plaintext is not valid UTF-8 text",
			run: decryptErrorRun(func() ([]byte, string) {
				return []byte{0xff}, contractPhrase
			}),
		},
		{
			name: "server rejected", exit: 6, code: "server_rejected",
			message: "the server rejected the request",
			run:     createResponse(http.StatusBadRequest, `{}`),
		},
		{
			name: "network unavailable", exit: 7, code: "network_unavailable",
			message: "the server could not be reached",
			run: func(t *testing.T) (int, string, string) {
				t.Helper()
				// The default client does not trust this test certificate, so the
				// connection fails before request headers can be transmitted.
				srv := httptest.NewTLSServer(http.NotFoundHandler())
				t.Cleanup(srv.Close)
				token := credentialFile(t, "token", contractToken+"\n")
				env, stdout, _ := contractEnv([]string{
					"burn", "--server", srv.URL, "--json", "--token-file", token, contractID,
				}, "")
				return Run(env), stdout.String(), srv.URL
			},
		},
		{
			name: "rate limited", exit: 7, code: "rate_limited",
			message:    "the server rate-limited the request",
			retryAfter: &retryAfter,
			run:        responseServer(http.StatusTooManyRequests, `{}`, retryAfter),
		},
		{
			name: "service unavailable", exit: 7, code: "service_unavailable",
			message: "the server is temporarily unavailable",
			run:     responseServer(http.StatusServiceUnavailable, `{}`),
		},
		{
			name: "unsupported secret", exit: 8, code: "unsupported_secret",
			message: "the ciphertext is not a supported Burnerpad passphrase secret",
			run: func(t *testing.T) (int, string, string) {
				t.Helper()
				path := filepath.Join(t.TempDir(), "truncated.blob")
				if err := os.WriteFile(path, append(envelope.EncodeToBytes([]byte{0}), '\n'), 0o600); err != nil {
					t.Fatal(err)
				}
				return commandErrorRun([]string{
					"decrypt", "--json", "--blob-file", path, "--passphrase-file", phraseFile(t),
				}, "", nil)(t)
			},
		},
		{
			name: "create outcome unknown", exit: 9, code: "create_outcome_unknown",
			message: "the request may have changed server state, but its outcome could not be confirmed",
			run:     createResponse(http.StatusOK, `{}`),
		},
		{
			name: "claim outcome unknown", exit: 9, code: "claim_outcome_unknown",
			message: "the request may have changed server state, but its outcome could not be confirmed",
			run: func(t *testing.T) (int, string, string) {
				t.Helper()
				srv, _ := contractResponseServer(t, http.StatusOK, `{}`)
				env, stdout, _ := contractEnv([]string{
					"reveal", "--json", "--passphrase-file", phraseFile(t), srv.URL + "/s/" + contractID,
				}, "")
				return Run(env), stdout.String(), srv.URL
			},
		},
		{
			name: "revoke outcome unknown", exit: 9, code: "revoke_outcome_unknown",
			message: "the request may have changed server state, but its outcome could not be confirmed",
			run:     responseServer(http.StatusOK, `{}`),
		},
		{
			name: "internal failure", exit: 10, code: "internal",
			message: "unexpected internal failure",
			run: commandErrorRun([]string{"create", "--json", "unexpected"}, "", func(env *Env) {
				env.Getenv = func(string) string { panic("test panic") }
			}),
		},
	}

	seen := make(map[string]bool)
	for _, test := range tests {
		if exit, ok := expectedMachineErrorExits[test.code]; !ok || exit != test.exit || seen[test.code] {
			t.Fatalf("test table contains duplicate or out-of-contract mapping %d/%s", test.exit, test.code)
		}
		seen[test.code] = true
	}
	for code, exit := range expectedMachineErrorExits {
		if !seen[code] {
			t.Errorf("test table does not exercise required mapping %d/%s", exit, code)
		}
	}
	if t.Failed() {
		return
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exit, stdout, server := test.run(t)
			if exit != test.exit {
				t.Fatalf("exit=%d, want %d; stdout=%s", exit, test.exit, stdout)
			}
			want := fmt.Sprintf(`{"status":"error","code":%q,"message":%q`, test.code, test.message)
			if server != "" {
				want += fmt.Sprintf(`,"server":%q`, server)
			}
			if test.retryAfter != nil {
				want += fmt.Sprintf(`,"retry_after":%d`, *test.retryAfter)
			}
			want += "}\n"
			if stdout != want {
				t.Fatalf("stdout=%q, want exact ordered JSON %q", stdout, want)
			}
		})
	}
}

func TestCurrentJSONRetryAfterContract(t *testing.T) {
	tests := []struct {
		name, retryAfter, code, message, optionalField string
		status                                         int
	}{
		{
			name:       "malformed rate limit header is omitted",
			status:     http.StatusTooManyRequests,
			retryAfter: "not-a-number",
			code:       "rate_limited",
			message:    "the server rate-limited the request",
		},
		{
			name:       "negative temporary failure header is omitted",
			status:     http.StatusServiceUnavailable,
			retryAfter: "-1",
			code:       "service_unavailable",
			message:    "the server is temporarily unavailable",
		},
		{
			name:       "plus sign is not delta-seconds",
			status:     http.StatusTooManyRequests,
			retryAfter: "+1",
			code:       "rate_limited",
			message:    "the server rate-limited the request",
		},
		{
			name:       "signed zero is not delta-seconds",
			status:     http.StatusServiceUnavailable,
			retryAfter: "-0",
			code:       "service_unavailable",
			message:    "the server is temporarily unavailable",
		},
		{
			name:       "overflowing delta-seconds is omitted",
			status:     http.StatusTooManyRequests,
			retryAfter: "9223372036854775808",
			code:       "rate_limited",
			message:    "the server rate-limited the request",
		},
		{
			name:          "zero is a valid rate limit delay",
			status:        http.StatusTooManyRequests,
			retryAfter:    "0",
			code:          "rate_limited",
			message:       "the server rate-limited the request",
			optionalField: `,"retry_after":0`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			srv, requests := contractResponseServerWithRetryAfter(t, test.status, `{}`, test.retryAfter)
			token := credentialFile(t, "token", contractToken+"\n")
			env, stdout, _ := contractEnv([]string{
				"burn", "--server", srv.URL, "--json", "--token-file", token, contractID,
			}, "")

			if exit := Run(env); exit != 7 {
				t.Fatalf("exit=%d, want 7; stdout=%s", exit, stdout)
			}
			if requests.Load() != 1 {
				t.Fatalf("requests=%d, want 1", requests.Load())
			}
			want := fmt.Sprintf(
				`{"status":"error","code":%q,"message":%q,"server":%q%s}`+"\n",
				test.code, test.message, srv.URL, test.optionalField,
			)
			if stdout.String() != want {
				t.Fatalf("stdout=%q, want exact ordered JSON %q", stdout, want)
			}
		})
	}
}

func TestCurrentProcessCreateHumanHandoff(t *testing.T) {
	const response = `{"id":"` + contractID + `","mgmt_token":"` + contractToken + `","ttl":75}`

	t.Run("interactive generated phrase", func(t *testing.T) {
		srv, requests := contractResponseServer(t, http.StatusOK, response)
		env, stdout, stderr := contractEnv([]string{"create", "--server", srv.URL + "/"}, "interactive plaintext")
		env.StdinTTY = true
		if exit := Run(env); exit != 0 {
			t.Fatalf("exit=%d; stdout=%s stderr=%s", exit, stdout, stderr)
		}
		if requests.Load() != 1 {
			t.Fatalf("requests=%d, want 1", requests.Load())
		}
		if want := srv.URL + "/s/" + contractID + "\n"; stdout.String() != want {
			t.Fatalf("stdout=%q, want link alone %q", stdout, want)
		}

		lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
		if len(lines) != 5 {
			t.Fatalf("stderr lines=%q, want prompt and four handoff lines", lines)
		}
		for index, want := range map[int]string{
			0: "Secret — type or paste text, then finish with EOF:",
			1: "burnerpad: server: " + srv.URL,
			3: "burnerpad: management token: " + contractToken,
			4: "burnerpad: effective ttl: 75s",
		} {
			if lines[index] != want {
				t.Errorf("stderr line %d=%q, want %q", index+1, lines[index], want)
			}
		}
		const phrasePrefix = "burnerpad: passphrase: "
		if !strings.HasPrefix(lines[2], phrasePrefix) {
			t.Fatalf("generated-phrase handoff line=%q", lines[2])
		}
		phrase := strings.TrimPrefix(lines[2], phrasePrefix)
		canonical, err := wordlist.Canonicalize([]byte(phrase))
		if err != nil || string(canonical) != phrase || len(strings.Fields(phrase)) != wordlist.PhraseWords {
			t.Fatalf("generated phrase=%q is not a canonical %d-word phrase: %v", phrase, wordlist.PhraseWords, err)
		}
	})

	t.Run("piped plaintext with supplied phrase", func(t *testing.T) {
		srv, requests := contractResponseServer(t, http.StatusOK, response)
		env, stdout, stderr := contractEnv([]string{
			"create", "--server", srv.URL + "/", "--passphrase-file", phraseFile(t),
		}, "piped plaintext")
		env.StdinPiped = true
		if exit := Run(env); exit != 0 {
			t.Fatalf("exit=%d; stdout=%s stderr=%s", exit, stdout, stderr)
		}
		if requests.Load() != 1 {
			t.Fatalf("requests=%d, want 1", requests.Load())
		}
		if want := srv.URL + "/s/" + contractID + "\n"; stdout.String() != want {
			t.Fatalf("stdout=%q, want link alone %q", stdout, want)
		}
		wantStderr := "burnerpad: server: " + srv.URL + "\n" +
			"burnerpad: management token: " + contractToken + "\n" +
			"burnerpad: effective ttl: 75s\n"
		if stderr.String() != wantStderr {
			t.Fatalf("stderr=%q, want handoff without supplied phrase %q", stderr, wantStderr)
		}
		if strings.Contains(stdout.String()+stderr.String(), contractPhrase) {
			t.Fatal("piped create repeated the caller-supplied phrase")
		}
	})

	t.Run("piped plaintext with generated phrase requires JSON", func(t *testing.T) {
		srv, requests := contractResponseServer(t, http.StatusOK, response)
		env, stdout, stderr := contractEnv([]string{"create", "--server", srv.URL}, "piped plaintext")
		env.StdinPiped = true
		if exit := Run(env); exit != 2 {
			t.Fatalf("exit=%d, want 2; stdout=%s stderr=%s", exit, stdout, stderr)
		}
		if requests.Load() != 0 {
			t.Fatalf("requests=%d, want no request before the JSON handoff check", requests.Load())
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout=%q, want empty", stdout)
		}
		const want = "burnerpad: invalid_option: a piped create with a generated passphrase requires --json\n"
		if stderr.String() != want {
			t.Fatalf("stderr=%q, want %q", stderr, want)
		}
	})
}

func commandErrorRun(args []string, stdin string, configure func(*Env)) jsonErrorRunner {
	return func(t *testing.T) (int, string, string) {
		t.Helper()
		env, stdout, _ := contractEnv(args, stdin)
		if configure != nil {
			configure(&env)
		}
		return Run(env), stdout.String(), ""
	}
}

func decryptErrorRun(input func() ([]byte, string)) jsonErrorRunner {
	return func(t *testing.T) (int, string, string) {
		t.Helper()
		plaintext, phrase := input()
		blob := envelope.EncryptPassphrase([]byte(contractPhrase), plaintext)
		path := filepath.Join(t.TempDir(), "secret.blob")
		if err := os.WriteFile(path, append(envelope.EncodeToBytes(blob), '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		phrasePath := credentialFile(t, "phrase", phrase+"\n")
		return commandErrorRun([]string{
			"decrypt", "--json", "--blob-file", path, "--passphrase-file", phrasePath,
		}, "", nil)(t)
	}
}

func contractResponseServer(t *testing.T, status int, body string, retryAfter ...int64) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	rawRetryAfter := ""
	if len(retryAfter) != 0 {
		rawRetryAfter = fmt.Sprint(retryAfter[0])
	}
	return contractResponseServerWithRetryAfter(t, status, body, rawRetryAfter)
}

func contractResponseServerWithRetryAfter(t *testing.T, status int, body, retryAfter string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}
