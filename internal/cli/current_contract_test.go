package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/burnerpad/burnerpad-cli/envelope"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

const (
	contractID     = "0123456789ABCDEFGHJKMNPQRS"
	contractToken  = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	contractPhrase = "aardvark carrot embroidery hardhat lyrics porcupine suave"
)

func contractEnv(args []string, stdin string) (Env, *bytes.Buffer, *bytes.Buffer) {
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	return Env{
		Args: args, Stdin: strings.NewReader(stdin), Stdout: stdout, Stderr: stderr,
		Getenv: func(string) string { return "" }, Version: "1.0.0", Commit: "test", Date: "test",
	}, stdout, stderr
}

func phraseFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "phrase")
	if err := os.WriteFile(p, []byte(contractPhrase+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlaintextDeliveryPreservesAuthenticatedBytesOutsideViewer(t *testing.T) {
	payload := []byte("before\x00\x1b]52;c;YQ==\a\r\nafter\u202e")

	t.Run("piped stdout", func(t *testing.T) {
		var stdout bytes.Buffer
		a := &application{env: Env{Stdout: &stdout}}
		if err := a.deliverPlaintext(&destination{}, "revealed", "https://example.com", payload); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(stdout.Bytes(), payload) {
			t.Fatalf("stdout = %q, want exact authenticated bytes %q", stdout.Bytes(), payload)
		}
	})

	t.Run("JSON", func(t *testing.T) {
		var stdout bytes.Buffer
		a := &application{env: Env{Stdout: &stdout}, cfg: config{json: true}}
		if err := a.deliverPlaintext(&destination{}, "revealed", "https://example.com", payload); err != nil {
			t.Fatal(err)
		}
		var got revealResult
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal([]byte(got.Plaintext), payload) {
			t.Fatalf("decoded plaintext = %q, want exact authenticated bytes %q", got.Plaintext, payload)
		}
	})

	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "plaintext")
		out, err := reserve(path)
		if err != nil {
			t.Fatal(err)
		}
		a := &application{env: Env{}}
		if err := a.deliverPlaintext(&destination{out: out}, "revealed", "https://example.com", payload); err != nil {
			out.discard()
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("file = %q, want exact authenticated bytes %q", got, payload)
		}
	})
}

func TestCurrentProcessCreateAndRevealInterop(t *testing.T) {
	var stored string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/secrets":
			var request struct {
				Blob string `json:"blob"`
				TTL  int64  `json:"ttl"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if r.Method != http.MethodPost || request.TTL != 90 {
				t.Fatalf("create request = %s %+v", r.Method, request)
			}
			stored = request.Blob
			w.Write([]byte(`{"id":"` + contractID + `","mgmt_token":"` + contractToken + `","ttl":60}`))
		case "/api/secrets/" + contractID + "/reveal":
			if r.Method != http.MethodPost {
				t.Fatalf("reveal method = %s", r.Method)
			}
			w.Write([]byte(`{"blob":"` + stored + `"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	pf := phraseFile(t)
	e, stdout, stderr := contractEnv([]string{"create", "--server", srv.URL, "--json", "--ttl", "90s", "--passphrase-file", pf}, "hello")
	if code := Run(e); code != 0 {
		t.Fatalf("create exit %d; stderr=%s", code, stderr)
	}
	wantCreate := `{"status":"created","server":"` + srv.URL + `","link":"` + srv.URL + `/s/` + contractID + `","phrase":"` + contractPhrase + `","mgmt_token":"` + contractToken + `","ttl":60}` + "\n"
	if stdout.String() != wantCreate {
		t.Fatalf("create stdout:\n%s\nwant:\n%s", stdout, wantCreate)
	}
	if !strings.Contains(stderr.String(), "server: "+srv.URL) {
		t.Fatalf("missing server disclosure: %s", stderr)
	}

	e, stdout, stderr = contractEnv([]string{"reveal", "--json", "--passphrase-file", pf, srv.URL + "/s/" + contractID}, "")
	if code := Run(e); code != 0 {
		t.Fatalf("reveal exit %d; stderr=%s", code, stderr)
	}
	wantReveal := `{"status":"revealed","server":"` + srv.URL + `","plaintext":"hello"}` + "\n"
	if stdout.String() != wantReveal {
		t.Fatalf("reveal stdout = %s", stdout)
	}
}

func TestCurrentProcessBurnReceipt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/secrets/"+contractID+"/burn" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"status":"burned"}`))
	}))
	defer srv.Close()
	receipt := `{"status":"created","server":"` + srv.URL + `","link":"` + srv.URL + `/s/` + contractID + `","phrase":"` + contractPhrase + `","mgmt_token":"` + contractToken + `","ttl":60}`
	e, stdout, stderr := contractEnv([]string{"burn", "--json"}, receipt)
	e.StdinPiped = true
	if code := Run(e); code != 0 {
		t.Fatalf("burn exit %d; stderr=%s", code, stderr)
	}
	want := `{"status":"burned","server":"` + srv.URL + `"}` + "\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %s", stdout)
	}
}

func TestCurrentProcessRejectsRetiredSurface(t *testing.T) {
	for _, args := range [][]string{{"open"}, {"report"}, {"reveal", "--force"}, {"create", "--words", "8"}} {
		e, _, _ := contractEnv(args, "")
		if code := Run(e); code != 2 {
			t.Errorf("Run(%v) = %d, want 2", args, code)
		}
	}
}

func TestCurrentProcessRejectsDuplicateOptions(t *testing.T) {
	e, _, _ := contractEnv([]string{"create", "--ttl", "60", "--ttl", "90"}, "secret")
	if code := Run(e); code != 2 {
		t.Fatalf("exit=%d, want 2", code)
	}
}

func TestCurrentProcessRejectsInapplicableGlobalOptions(t *testing.T) {
	for _, args := range [][]string{
		{"version", "--quiet"},
		{"words", "--timeout", "1s"},
		{"decrypt", "--timeout", "1s", "--blob-file", "blob"},
	} {
		e, _, _ := contractEnv(args, "")
		if code := Run(e); code != 2 {
			t.Fatalf("Run(%v)=%d, want 2", args, code)
		}
	}
}

func TestCurrentProcessRejectsMixedPrimaryInputs(t *testing.T) {
	for _, args := range [][]string{
		{"reveal", "https://example.com/s/" + contractID},
		{"burn", contractID},
	} {
		e, _, _ := contractEnv(args, "piped input")
		e.StdinPiped = true
		if code := Run(e); code != 2 {
			t.Fatalf("Run(%v)=%d, want 2", args, code)
		}
	}
}

func TestCurrentProcessPreflightErrorsAreSecretFreeAndCarrySelectedServer(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer srv.Close()
	badPhrase := filepath.Join(t.TempDir(), "bad-phrase")
	if err := os.WriteFile(badPhrase, []byte("not a compatible phrase\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e, stdout, stderr := contractEnv([]string{"reveal", "--json", "--passphrase-file", badPhrase, srv.URL + "/s/" + contractID}, "")
	if code := Run(e); code != 2 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["code"] != "invalid_credential_source" || result["server"] != srv.URL {
		t.Fatalf("error JSON=%s", stdout)
	}
	if requests.Load() != 0 {
		t.Fatalf("preflight error sent %d requests", requests.Load())
	}
	for _, forbidden := range []string{badPhrase, "not a compatible phrase", contractID} {
		if strings.Contains(stdout.String()+stderr.String(), forbidden) {
			t.Fatalf("diagnostic leaked %q", forbidden)
		}
	}
}

func TestCurrentProcessDoesNotSendWhenServerDisclosureFails(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer srv.Close()
	e, _, _ := contractEnv([]string{"create", "--server", srv.URL, "--json", "--passphrase-file", phraseFile(t)}, "secret")
	e.Stderr = failingWriter{}
	if code := Run(e); code != 3 {
		t.Fatalf("exit=%d", code)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests=%d, want 0", requests.Load())
	}
}

func TestCurrentProcessValidatesFullURLOriginBeforeReadingToken(t *testing.T) {
	e, stdout, _ := contractEnv([]string{
		"burn", "--json", "--token-file", filepath.Join(t.TempDir(), "missing"),
		"http://example.com/s/" + contractID,
	}, "")
	if code := Run(e); code != 2 {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout.String(), `"code":"invalid_input"`) {
		t.Fatalf("stdout=%s", stdout)
	}
}

func TestCurrentProcessRequiresCreatedReceiptStatus(t *testing.T) {
	receipt := `{"status":"error","server":"http://127.0.0.1:4000","link":"http://127.0.0.1:4000/s/` + contractID + `","mgmt_token":"` + contractToken + `"}`
	e, _, _ := contractEnv([]string{"burn", "--json"}, receipt)
	e.StdinPiped = true
	if code := Run(e); code != 2 {
		t.Fatalf("exit=%d", code)
	}
}

func TestCurrentProcessDecryptCanonicalBlob(t *testing.T) {
	blob := envelope.EncryptPassphrase([]byte(contractPhrase), []byte("offline"))
	p := filepath.Join(t.TempDir(), "blob")
	if err := os.WriteFile(p, append(envelope.EncodeToBytes(blob), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	e, stdout, stderr := contractEnv([]string{"decrypt", "--json", "--blob-file", p, "--passphrase-file", phraseFile(t)}, "")
	if code := Run(e); code != 0 {
		t.Fatalf("exit %d; stderr=%s", code, stderr)
	}
	if stdout.String() != "{\"status\":\"decrypted\",\"plaintext\":\"offline\"}\n" {
		t.Fatalf("stdout=%s", stdout)
	}
}

func TestCurrentProcessRevealRecoverySurvivesWrongPhrase(t *testing.T) {
	blob := envelope.EncryptPassphrase([]byte(contractPhrase), []byte("recover me"))
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Write([]byte(`{"blob":"` + string(envelope.EncodeToBytes(blob)) + `"}`))
	}))
	defer srv.Close()
	wrong := filepath.Join(t.TempDir(), "wrong-phrase")
	if err := os.WriteFile(wrong, []byte("freeway faucet unnoticed energy emoticon elves enormous\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recovery := filepath.Join(t.TempDir(), "claimed.blob")
	e, stdout, stderr := contractEnv([]string{"reveal", "--json", "--passphrase-file", wrong, "--keep-blob", recovery, srv.URL + "/s/" + contractID}, "")
	if code := Run(e); code != 5 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests=%d", requests.Load())
	}
	wantRecovery := string(envelope.EncodeToBytes(blob)) + "\n"
	got, err := os.ReadFile(recovery)
	if err != nil || string(got) != wantRecovery {
		t.Fatalf("recovery=%q err=%v", got, err)
	}
	var result map[string]any
	if json.Unmarshal(stdout.Bytes(), &result) != nil || result["code"] != "passphrase_failed" || result["server"] != srv.URL {
		t.Fatalf("error JSON=%s", stdout)
	}
	for _, secretValue := range []string{contractID, contractPhrase, "recover me", string(envelope.EncodeToBytes(blob))} {
		if strings.Contains(stdout.String()+stderr.String(), secretValue) {
			t.Fatalf("diagnostic leaked secret material %q", secretValue)
		}
	}
}

func TestCurrentProcessDestinationPreflightPreventsClaim(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer srv.Close()
	existing := filepath.Join(t.TempDir(), "existing")
	if err := os.WriteFile(existing, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	e, _, _ := contractEnv([]string{"reveal", "--passphrase-file", phraseFile(t), "--out", existing, srv.URL + "/s/" + contractID}, "")
	if code := Run(e); code != 3 {
		t.Fatalf("exit=%d", code)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests=%d, want 0", requests.Load())
	}
	if got, _ := os.ReadFile(existing); string(got) != "keep" {
		t.Fatalf("existing output changed: %q", got)
	}
}

func TestCurrentProcessRevealIgnoresExplicitServer(t *testing.T) {
	blob := envelope.EncryptPassphrase([]byte(contractPhrase), []byte("origin wins"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"blob":"` + string(envelope.EncodeToBytes(blob)) + `"}`))
	}))
	defer srv.Close()
	e, stdout, stderr := contractEnv([]string{"reveal", "--json", "--server", "https://example.invalid", "--passphrase-file", phraseFile(t), srv.URL + "/s/" + contractID}, "")
	if code := Run(e); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr.String(), "--server is ignored") || !strings.Contains(stdout.String(), `"server":"`+srv.URL+`"`) {
		t.Fatalf("stdout=%s stderr=%s", stdout, stderr)
	}
}

func TestCurrentCompletionCommandMatchesShippedArtifacts(t *testing.T) {
	files := map[string]string{
		"bash":       "burnerpad.bash",
		"zsh":        "burnerpad.zsh",
		"fish":       "burnerpad.fish",
		"powershell": "burnerpad.ps1",
	}
	for shell, name := range files {
		t.Run(shell, func(t *testing.T) {
			artifact, err := os.ReadFile(filepath.Join("..", "..", "completions", name))
			if err != nil {
				t.Fatal(err)
			}
			e, stdout, stderr := contractEnv([]string{"completion", shell}, "")
			if code := Run(e); code != 0 {
				t.Fatalf("exit=%d stderr=%s", code, stderr)
			}
			if !bytes.Equal(stdout.Bytes(), artifact) {
				t.Fatalf("completion command and %s have drifted", name)
			}
		})
	}
}
