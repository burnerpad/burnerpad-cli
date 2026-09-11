package cli

import (
	"bytes"
	"encoding/json"
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

type sensitiveValueClass string

const (
	wrongPhraseCanary       = "freeway faucet unnoticed energy emoticon elves enormous"
	createPlaintextCanary   = "diagnostic create plaintext canary"
	invalidPlaintextCanary  = "diagnostic invalid authenticated plaintext canary"
	claimPlaintextCanary    = "diagnostic claim plaintext canary"
	truncatedResponseCanary = "diagnostic-truncated-response-canary"
)

const (
	sensitiveWrongPhrase       sensitiveValueClass = "wrong_phrase"
	sensitiveCorrectPhrase     sensitiveValueClass = "correct_phrase"
	sensitiveManagementToken   sensitiveValueClass = "management_token"
	sensitivePlaintext         sensitiveValueClass = "plaintext"
	sensitiveCiphertext        sensitiveValueClass = "ciphertext"
	sensitiveCompleteResponse  sensitiveValueClass = "complete_response"
	sensitiveTruncatedResponse sensitiveValueClass = "truncated_response"
	sensitiveFullShareURL      sensitiveValueClass = "full_share_url"
	sensitiveIdentifier        sensitiveValueClass = "identifier"
	sensitiveFilesystemPath    sensitiveValueClass = "filesystem_path"
	sensitiveNestedCause       sensitiveValueClass = "nested_cause"
)

var expectedRealRunSensitiveValueClasses = map[sensitiveValueClass]struct{}{
	sensitiveWrongPhrase:       {},
	sensitiveCorrectPhrase:     {},
	sensitiveManagementToken:   {},
	sensitivePlaintext:         {},
	sensitiveCiphertext:        {},
	sensitiveCompleteResponse:  {},
	sensitiveTruncatedResponse: {},
	sensitiveFullShareURL:      {},
	sensitiveIdentifier:        {},
	sensitiveFilesystemPath:    {},
}

type sensitiveValue struct {
	class sensitiveValueClass
	value string
}

type diagnosticObservation struct {
	exit           int
	stdout, stderr string
	wantStderr     string
	server         string
	sensitive      []sensitiveValue
}

type jsonErrorRunner func(*testing.T) diagnosticObservation

type receivedRequest struct {
	method, path, body string
	responseBytes      int
	err                error
}

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
			t.Errorf("report(%q/%d) stdout_match=false stdout_len=%d", command.code, command.exit, stdout.Len())
		}
	}
}

func TestReportNeverTraversesNestedCauses(t *testing.T) {
	const (
		message = "reviewed machine error"
		canary  = "diagnostic-nested-cause-canary"
	)
	if sensitiveNestedCause != "nested_cause" {
		t.Fatalf("nested-cause sensitive class changed to %q", sensitiveNestedCause)
	}
	for code, exit := range machineErrorExits {
		for _, jsonMode := range []bool{false, true} {
			mode := "human"
			if jsonMode {
				mode = "json"
			}
			t.Run(mode+"/"+code, func(t *testing.T) {
				var stdout, stderr bytes.Buffer
				nested := fmt.Errorf("inner %s", canary)
				command := commandError{
					exit: exit, code: code, message: message,
					cause: fmt.Errorf("outer cause: %w", nested),
				}
				err := fmt.Errorf("command wrapper: %w", command)
				a := application{
					env: Env{Stdout: &stdout, Stderr: &stderr},
					cfg: config{json: jsonMode},
				}
				if got := a.report(err); got != exit {
					t.Fatalf("report exit=%d, want %d", got, exit)
				}
				wantStdout, wantStderr := "", fmt.Sprintf("burnerpad: %s: %s\n", code, message)
				if jsonMode {
					wantStdout = fmt.Sprintf(`{"status":"error","code":%q,"message":%q}`+"\n", code, message)
					wantStderr = ""
				}
				if strings.Contains(stdout.String()+stderr.String(), canary) {
					t.Fatalf("code=%s mode=%s diagnostic disclosed nested cause", code, mode)
				}
				if stdout.String() != wantStdout || stderr.String() != wantStderr {
					t.Fatalf("code=%s mode=%s stdout_match=%t stdout_len=%d stderr_match=%t stderr_len=%d",
						code, mode, stdout.String() == wantStdout, stdout.Len(), stderr.String() == wantStderr, stderr.Len())
				}
			})
		}
	}
}

func TestCurrentJSONErrorVocabularyAndExitMapping(t *testing.T) {
	responseServer := func(status int, body string, retryAfter ...int64) jsonErrorRunner {
		return func(t *testing.T) diagnosticObservation {
			t.Helper()
			rawRetryAfter := ""
			if len(retryAfter) != 0 {
				rawRetryAfter = fmt.Sprint(retryAfter[0])
			}
			srv, received := recordedResponseServer(t, status, body, rawRetryAfter, 0)
			tokenPath := credentialFile(t, "diagnostic-token-file-path-canary", contractToken+"\n")
			env, stdout, stderr := contractEnv([]string{
				"burn", "--server", srv.URL, "--json", "--token-file", tokenPath, contractID,
			}, "")
			observation := diagnosticObservation{
				exit: Run(env), stdout: stdout.String(), stderr: stderr.String(),
				wantStderr: "burnerpad: server: " + srv.URL + "\n", server: srv.URL,
			}
			request := takeReceivedRequest(t, received, "burn")
			wantPath := "/api/secrets/" + contractID + "/burn"
			wantBody := `{"mgmt_token":"` + contractToken + `"}`
			if request.err != nil || request.method != http.MethodPost || request.path != wantPath ||
				request.body != wantBody || request.responseBytes != len(body) {
				t.Fatal("burn response fixture did not observe the expected complete request and response")
			}
			observation.sensitive = []sensitiveValue{
				{class: sensitiveManagementToken, value: contractToken},
				{class: sensitiveIdentifier, value: contractID},
			}
			observation.sensitive = append(observation.sensitive, diagnosticPathValues(tokenPath)...)
			if status == http.StatusOK {
				observation.sensitive = append(observation.sensitive, diagnosticResponseValues(sensitiveCompleteResponse, body)...)
			}
			return observation
		}
	}
	createResponse := func(status int, body string) jsonErrorRunner {
		return func(t *testing.T) diagnosticObservation {
			t.Helper()
			srv, received := recordedResponseServer(t, status, body, "", 0)
			phrasePath := credentialFile(t, "diagnostic-create-phrase-path-canary", contractPhrase+"\n")
			env, stdout, stderr := contractEnv([]string{
				"create", "--server", srv.URL, "--json", "--passphrase-file", phrasePath,
			}, createPlaintextCanary)
			env.StdinPiped = true
			observation := diagnosticObservation{
				exit: Run(env), stdout: stdout.String(), stderr: stderr.String(),
				wantStderr: "burnerpad: server: " + srv.URL + "\n", server: srv.URL,
			}
			request := takeReceivedRequest(t, received, "create")
			var payload struct {
				Blob string `json:"blob"`
			}
			if request.err != nil || request.method != http.MethodPost || request.path != "/api/secrets" ||
				request.responseBytes != len(body) || json.Unmarshal([]byte(request.body), &payload) != nil || payload.Blob == "" {
				t.Fatal("create response fixture did not observe the expected complete request and response")
			}
			decoded, err := envelope.DecodeCanonical([]byte(payload.Blob))
			if err != nil {
				t.Fatal("create request ciphertext was not canonical base64url")
			}
			rawCiphertext := string(decoded)
			clear(decoded)
			observation.sensitive = append(observation.sensitive, diagnosticPhraseValues(sensitiveCorrectPhrase, contractPhrase)...)
			observation.sensitive = append(observation.sensitive, diagnosticPathValues(phrasePath)...)
			observation.sensitive = append(observation.sensitive,
				sensitiveValue{class: sensitivePlaintext, value: createPlaintextCanary},
				sensitiveValue{class: sensitiveCiphertext, value: payload.Blob},
				sensitiveValue{class: sensitiveCiphertext, value: rawCiphertext},
			)
			if status == http.StatusOK {
				observation.sensitive = append(observation.sensitive, diagnosticResponseValues(sensitiveCompleteResponse, body)...)
			}
			return observation
		}
	}

	retryAfter := int64(17)
	tests := []struct {
		name, code, message string
		exit                int
		retryAfter          *int64
		expectedClasses     []sensitiveValueClass
		run                 jsonErrorRunner
	}{
		{
			name: "invalid command", exit: 2, code: "invalid_command",
			message: "a command is required; run 'burnerpad help'", expectedClasses: nil,
			run: commandErrorRun([]string{"--json"}, "", nil),
		},
		{
			name: "invalid option", exit: 2, code: "invalid_option",
			message: "invalid or unsupported option", expectedClasses: nil,
			run: commandErrorRun([]string{"create", "--json", "--not-an-option"}, "", nil),
		},
		{
			name: "invalid input", exit: 2, code: "invalid_input",
			message: "create does not accept positional arguments", expectedClasses: nil,
			run: commandErrorRun([]string{"create", "--json", "unexpected"}, "", nil),
		},
		{
			name: "invalid credential source", exit: 2, code: "invalid_credential_source",
			message: "passphrase files must be named and passphrase descriptors must be 3 or greater", expectedClasses: nil,
			run: commandErrorRun([]string{"create", "--json", "--passphrase-file", "-"}, "", func(env *Env) {
				env.StdinPiped = true
			}),
		},
		{
			name: "local I/O failure", exit: 3, code: "local_io_failed",
			message:         "cannot read the ciphertext file",
			expectedClasses: []sensitiveValueClass{sensitiveCorrectPhrase, sensitiveFilesystemPath},
			run: func(t *testing.T) diagnosticObservation {
				t.Helper()
				missing := filepath.Join(t.TempDir(), "diagnostic-missing-ciphertext-path-canary")
				phrasePath := credentialFile(t, "diagnostic-missing-run-phrase-path-canary", contractPhrase+"\n")
				observation := commandErrorRun([]string{
					"decrypt", "--json", "--blob-file", missing, "--passphrase-file", phrasePath,
				}, "", nil)(t)
				observation.sensitive = append(observation.sensitive, diagnosticPhraseValues(sensitiveCorrectPhrase, contractPhrase)...)
				observation.sensitive = append(observation.sensitive, diagnosticPathValues(missing)...)
				observation.sensitive = append(observation.sensitive, diagnosticPathValues(phrasePath)...)
				return observation
			},
		},
		{
			name: "secret unavailable", exit: 4, code: "secret_unavailable",
			message:         "the secret is unavailable",
			expectedClasses: []sensitiveValueClass{sensitiveManagementToken, sensitiveIdentifier, sensitiveFilesystemPath},
			run:             responseServer(http.StatusNotFound, `"diagnostic-secret-unavailable-response-canary"`),
		},
		{
			name: "passphrase failed", exit: 5, code: "passphrase_failed",
			message:         "the passphrase did not open the secret",
			expectedClasses: []sensitiveValueClass{sensitiveWrongPhrase, sensitiveCiphertext, sensitiveFilesystemPath},
			run: decryptErrorRun(
				nil, contractPhrase, wrongPhraseCanary, false,
			),
		},
		{
			name: "plaintext invalid", exit: 5, code: "plaintext_invalid",
			message:         "the authenticated plaintext is not valid UTF-8 text",
			expectedClasses: []sensitiveValueClass{sensitiveCorrectPhrase, sensitivePlaintext, sensitiveCiphertext, sensitiveFilesystemPath},
			run: decryptErrorRun(
				append([]byte{0xff}, []byte(invalidPlaintextCanary)...), contractPhrase, contractPhrase, true,
			),
		},
		{
			name: "server rejected", exit: 6, code: "server_rejected",
			message:         "the server rejected the request",
			expectedClasses: []sensitiveValueClass{sensitiveCorrectPhrase, sensitivePlaintext, sensitiveCiphertext, sensitiveFilesystemPath},
			run:             createResponse(http.StatusBadRequest, `"diagnostic-create-rejected-response-canary"`),
		},
		{
			name: "network unavailable", exit: 7, code: "network_unavailable",
			message:         "the server could not be reached",
			expectedClasses: []sensitiveValueClass{sensitiveManagementToken, sensitiveIdentifier, sensitiveFilesystemPath, sensitiveFullShareURL},
			run: func(t *testing.T) diagnosticObservation {
				t.Helper()
				// The default client does not trust this test certificate, so the
				// connection fails before request headers can be transmitted.
				srv := httptest.NewTLSServer(http.NotFoundHandler())
				t.Cleanup(srv.Close)
				tokenPath := credentialFile(t, "diagnostic-network-token-path-canary", contractToken+"\n")
				shareURL := srv.URL + "/s/" + contractID
				env, stdout, stderr := contractEnv([]string{
					"burn", "--json", "--token-file", tokenPath, shareURL,
				}, "")
				observation := diagnosticObservation{
					exit: Run(env), stdout: stdout.String(), stderr: stderr.String(), server: srv.URL,
					wantStderr: "burnerpad: warning: the share URL is now present in shell history\n" +
						"burnerpad: server: " + srv.URL + "\n",
					sensitive: []sensitiveValue{
						{class: sensitiveManagementToken, value: contractToken},
						{class: sensitiveIdentifier, value: contractID},
						{class: sensitiveFullShareURL, value: shareURL},
					},
				}
				observation.sensitive = append(observation.sensitive, diagnosticPathValues(tokenPath)...)
				return observation
			},
		},
		{
			name: "rate limited", exit: 7, code: "rate_limited",
			message:         "the server rate-limited the request",
			retryAfter:      &retryAfter,
			expectedClasses: []sensitiveValueClass{sensitiveManagementToken, sensitiveIdentifier, sensitiveFilesystemPath},
			run:             responseServer(http.StatusTooManyRequests, `"diagnostic-rate-limited-response-canary"`, retryAfter),
		},
		{
			name: "service unavailable", exit: 7, code: "service_unavailable",
			message:         "the server is temporarily unavailable",
			expectedClasses: []sensitiveValueClass{sensitiveManagementToken, sensitiveIdentifier, sensitiveFilesystemPath},
			run:             responseServer(http.StatusServiceUnavailable, `"diagnostic-service-unavailable-response-canary"`),
		},
		{
			name: "unsupported secret", exit: 8, code: "unsupported_secret",
			message:         "the ciphertext is not a supported Burnerpad passphrase secret",
			expectedClasses: []sensitiveValueClass{sensitiveCorrectPhrase, sensitiveCiphertext, sensitiveFilesystemPath},
			run: func(t *testing.T) diagnosticObservation {
				t.Helper()
				invalidBlob := []byte("diagnostic unsupported ciphertext canary")
				encoded := string(envelope.EncodeToBytes(invalidBlob))
				blobPath := filepath.Join(t.TempDir(), "diagnostic-unsupported-ciphertext-path-canary")
				if err := os.WriteFile(blobPath, []byte(encoded+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				phrasePath := credentialFile(t, "diagnostic-unsupported-phrase-path-canary", contractPhrase+"\n")
				observation := commandErrorRun([]string{
					"decrypt", "--json", "--blob-file", blobPath, "--passphrase-file", phrasePath,
				}, "", nil)(t)
				observation.sensitive = append(observation.sensitive, diagnosticPhraseValues(sensitiveCorrectPhrase, contractPhrase)...)
				observation.sensitive = append(observation.sensitive, diagnosticPathValues(blobPath)...)
				observation.sensitive = append(observation.sensitive, diagnosticPathValues(phrasePath)...)
				observation.sensitive = append(observation.sensitive,
					sensitiveValue{class: sensitiveCiphertext, value: string(invalidBlob)},
					sensitiveValue{class: sensitiveCiphertext, value: encoded},
				)
				return observation
			},
		},
		{
			name: "create outcome unknown", exit: 9, code: "create_outcome_unknown",
			message:         "the request may have changed server state, but its outcome could not be confirmed",
			expectedClasses: []sensitiveValueClass{sensitiveCorrectPhrase, sensitivePlaintext, sensitiveCiphertext, sensitiveFilesystemPath, sensitiveCompleteResponse},
			run:             createResponse(http.StatusOK, `"diagnostic-create-unknown-response-canary"`),
		},
		{
			name: "claim outcome unknown", exit: 9, code: "claim_outcome_unknown",
			message:         "the request may have changed server state, but its outcome could not be confirmed",
			expectedClasses: []sensitiveValueClass{sensitiveCorrectPhrase, sensitiveCiphertext, sensitiveFilesystemPath, sensitiveTruncatedResponse, sensitiveFullShareURL, sensitiveIdentifier},
			run: func(t *testing.T) diagnosticObservation {
				t.Helper()
				validBlob := envelope.EncryptPassphrase([]byte(contractPhrase), []byte(claimPlaintextCanary))
				encoded := string(envelope.EncodeToBytes(validBlob))
				responseBody := fmt.Sprintf(`{"blob":%q,"unexpected":%q}`, encoded, truncatedResponseCanary)
				var positive struct {
					Blob string `json:"blob"`
				}
				decodeJSONErr := json.Unmarshal([]byte(responseBody), &positive)
				decoded, decodeBlobErr := envelope.DecodeCanonical([]byte(positive.Blob))
				opened, openErr := envelope.DecryptPassphrase(decoded, []byte(contractPhrase))
				positiveOK := decodeJSONErr == nil && decodeBlobErr == nil && openErr == nil &&
					bytes.Equal(opened, []byte(claimPlaintextCanary))
				clear(decoded)
				clear(opened)
				if !positiveOK {
					t.Fatal("claim truncation positive control did not form a valid current-contract response")
				}

				srv, served := recordedResponseServer(t, http.StatusOK, responseBody, "", 1)
				phrasePath := credentialFile(t, "diagnostic-claim-phrase-path-canary", contractPhrase+"\n")
				shareURL := srv.URL + "/s/" + contractID
				env, stdout, stderr := contractEnv([]string{
					"reveal", "--json", "--passphrase-file", phrasePath, shareURL,
				}, "")
				observation := diagnosticObservation{
					exit: Run(env), stdout: stdout.String(), stderr: stderr.String(), server: srv.URL,
					wantStderr: "burnerpad: warning: the share URL is now present in shell history\n" +
						"burnerpad: server: " + srv.URL + "\n",
					sensitive: []sensitiveValue{
						{class: sensitiveFullShareURL, value: shareURL},
						{class: sensitiveIdentifier, value: contractID},
						{class: sensitiveCiphertext, value: string(validBlob)},
						{class: sensitiveCiphertext, value: encoded},
						{class: sensitiveTruncatedResponse, value: responseBody},
					},
				}
				observation.sensitive = append(observation.sensitive, diagnosticPhraseValues(sensitiveCorrectPhrase, contractPhrase)...)
				observation.sensitive = append(observation.sensitive, diagnosticPathValues(phrasePath)...)
				request := takeReceivedRequest(t, served, "claim")
				wantPath := "/api/secrets/" + contractID + "/reveal"
				if request.err != nil || request.method != http.MethodPost || request.path != wantPath ||
					request.body != `{}` || request.responseBytes != len(responseBody) {
					t.Fatal("claim truncation fixture did not observe the expected complete request and partial response")
				}
				return observation
			},
		},
		{
			name: "revoke outcome unknown", exit: 9, code: "revoke_outcome_unknown",
			message:         "the request may have changed server state, but its outcome could not be confirmed",
			expectedClasses: []sensitiveValueClass{sensitiveManagementToken, sensitiveIdentifier, sensitiveFilesystemPath, sensitiveCompleteResponse},
			run:             responseServer(http.StatusOK, `"diagnostic-revoke-unknown-response-canary"`),
		},
		{
			name: "internal failure", exit: 10, code: "internal",
			message: "unexpected internal failure", expectedClasses: nil,
			run: func(t *testing.T) diagnosticObservation {
				t.Helper()
				observation := commandErrorRun([]string{"create", "--json", "unexpected"}, "", func(env *Env) {
					env.Getenv = func(string) string { panic("test panic") }
				})(t)
				return observation
			},
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

	seenSensitiveClasses := make(map[sensitiveValueClass]bool)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation := test.run(t)
			combined := observation.stdout + observation.stderr
			observedClasses := make(map[sensitiveValueClass]bool)
			for index, sensitive := range observation.sensitive {
				if sensitive.value == "" {
					t.Fatalf("code=%s class=%s needle=%d is empty", test.code, sensitive.class, index)
				}
				if _, ok := expectedRealRunSensitiveValueClasses[sensitive.class]; !ok {
					t.Fatalf("code=%s has unexpected sensitive class %s", test.code, sensitive.class)
				}
				observedClasses[sensitive.class] = true
				seenSensitiveClasses[sensitive.class] = true
				if strings.Contains(combined, sensitive.value) {
					t.Fatalf("code=%s leaked class=%s needle=%d stdout_len=%d stderr_len=%d",
						test.code, sensitive.class, index, len(observation.stdout), len(observation.stderr))
				}
			}

			expectedClasses := make(map[sensitiveValueClass]bool)
			for _, class := range test.expectedClasses {
				if _, ok := expectedRealRunSensitiveValueClasses[class]; !ok {
					t.Fatalf("code=%s expects unknown sensitive class %s", test.code, class)
				}
				if expectedClasses[class] {
					t.Fatalf("code=%s repeats expected sensitive class %s", test.code, class)
				}
				expectedClasses[class] = true
			}
			for class := range observedClasses {
				if !expectedClasses[class] {
					t.Fatalf("code=%s observed undeclared sensitive class %s", test.code, class)
				}
			}
			for class := range expectedClasses {
				if !observedClasses[class] {
					t.Fatalf("code=%s did not observe expected sensitive class %s", test.code, class)
				}
			}

			want := fmt.Sprintf(`{"status":"error","code":%q,"message":%q`, test.code, test.message)
			if observation.server != "" {
				want += fmt.Sprintf(`,"server":%q`, observation.server)
			}
			if test.retryAfter != nil {
				want += fmt.Sprintf(`,"retry_after":%d`, *test.retryAfter)
			}
			want += "}\n"
			stdoutMatch := observation.stdout == want
			stderrMatch := observation.stderr == observation.wantStderr
			if observation.exit != test.exit || !stdoutMatch || !stderrMatch {
				t.Fatalf("code=%s exit=%d want_exit=%d stdout_match=%t stdout_len=%d stderr_match=%t stderr_len=%d",
					test.code, observation.exit, test.exit, stdoutMatch, len(observation.stdout), stderrMatch, len(observation.stderr))
			}
		})
	}
	for class := range expectedRealRunSensitiveValueClasses {
		if !seenSensitiveClasses[class] {
			t.Errorf("real-Run table does not cover sensitive-value class %q", class)
		}
	}
	for class := range seenSensitiveClasses {
		if _, ok := expectedRealRunSensitiveValueClasses[class]; !ok {
			t.Errorf("real-Run table covered unexpected sensitive-value class %q", class)
		}
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

func diagnosticPathValues(path string) []sensitiveValue {
	return []sensitiveValue{
		{class: sensitiveFilesystemPath, value: path},
		{class: sensitiveFilesystemPath, value: filepath.Base(path)},
	}
}

func diagnosticPhraseValues(class sensitiveValueClass, phrase string) []sensitiveValue {
	values := []sensitiveValue{{class: class, value: phrase}}
	for _, word := range strings.Fields(phrase) {
		values = append(values, sensitiveValue{class: class, value: word})
	}
	return values
}

func diagnosticResponseValues(class sensitiveValueClass, body string) []sensitiveValue {
	return []sensitiveValue{{class: class, value: body}}
}

func recordedResponseServer(t *testing.T, status int, body, retryAfter string, extraContentLength int) (*httptest.Server, <-chan receivedRequest) {
	t.Helper()
	received := make(chan receivedRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestBody, requestErr := io.ReadAll(r.Body)
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		if extraContentLength != 0 {
			w.Header().Set("Content-Length", fmt.Sprint(len(body)+extraContentLength))
			w.Header().Set("Connection", "close")
		}
		w.WriteHeader(status)
		written, writeErr := io.WriteString(w, body)
		if requestErr != nil {
			writeErr = requestErr
		} else if writeErr == nil && written != len(body) {
			writeErr = fmt.Errorf("short fixture write")
		}
		received <- receivedRequest{
			method: r.Method, path: r.URL.Path, body: string(requestBody),
			responseBytes: written, err: writeErr,
		}
	}))
	t.Cleanup(srv.Close)
	return srv, received
}

func takeReceivedRequest(t *testing.T, received <-chan receivedRequest, operation string) receivedRequest {
	t.Helper()
	select {
	case request := <-received:
		return request
	default:
		t.Fatalf("%s request did not reach its response fixture", operation)
		return receivedRequest{}
	}
}

func commandErrorRun(args []string, stdin string, configure func(*Env)) jsonErrorRunner {
	return func(t *testing.T) diagnosticObservation {
		t.Helper()
		env, stdout, stderr := contractEnv(args, stdin)
		if configure != nil {
			configure(&env)
		}
		return diagnosticObservation{exit: Run(env), stdout: stdout.String(), stderr: stderr.String()}
	}
}

func decryptErrorRun(plaintext []byte, encryptionPhrase, suppliedPhrase string, authenticatedPlaintext bool) jsonErrorRunner {
	return func(t *testing.T) diagnosticObservation {
		t.Helper()
		blob := envelope.EncryptPassphrase([]byte(encryptionPhrase), plaintext)
		encoded := string(envelope.EncodeToBytes(blob))
		blobPath := filepath.Join(t.TempDir(), "diagnostic-ciphertext-file-path-canary")
		if err := os.WriteFile(blobPath, []byte(encoded+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		phrasePath := credentialFile(t, "diagnostic-decrypt-phrase-path-canary", suppliedPhrase+"\n")
		observation := commandErrorRun([]string{
			"decrypt", "--json", "--blob-file", blobPath, "--passphrase-file", phrasePath,
		}, "", nil)(t)
		if suppliedPhrase == encryptionPhrase {
			observation.sensitive = append(observation.sensitive, diagnosticPhraseValues(sensitiveCorrectPhrase, encryptionPhrase)...)
		} else {
			observation.sensitive = append(observation.sensitive, diagnosticPhraseValues(sensitiveWrongPhrase, suppliedPhrase)...)
		}
		observation.sensitive = append(observation.sensitive, diagnosticPathValues(blobPath)...)
		observation.sensitive = append(observation.sensitive, diagnosticPathValues(phrasePath)...)
		observation.sensitive = append(observation.sensitive,
			sensitiveValue{class: sensitiveCiphertext, value: string(blob)},
			sensitiveValue{class: sensitiveCiphertext, value: encoded},
		)
		if authenticatedPlaintext {
			observation.sensitive = append(observation.sensitive, sensitiveValue{class: sensitivePlaintext, value: string(plaintext)})
			if len(plaintext) > 1 {
				observation.sensitive = append(observation.sensitive, sensitiveValue{class: sensitivePlaintext, value: string(plaintext[1:])})
			}
		}
		return observation
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
