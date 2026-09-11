package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

const signalTestTimeout = 5 * time.Second

func TestRunPreloadedSignalPreventsMutation(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Write([]byte(`{"id":"` + contractID + `","mgmt_token":"` + contractToken + `","ttl":60}`))
	}))
	defer srv.Close()

	for _, test := range []struct {
		name string
		sig  os.Signal
		exit int
	}{
		{name: "SIGINT", sig: os.Interrupt, exit: 130},
		{name: "SIGTERM", sig: syscall.SIGTERM, exit: 143},
	} {
		t.Run(test.name, func(t *testing.T) {
			signals := make(chan os.Signal, 1)
			signals <- test.sig
			e, stdout, stderr := contractEnv([]string{
				"create", "--server", srv.URL, "--json", "--passphrase-file", phraseFile(t),
			}, "must not be created")
			e.Signals = signals

			if code := Run(e); code != test.exit {
				t.Fatalf("exit=%d, want %d; stdout=%s stderr=%s", code, test.exit, stdout, stderr)
			}
			if requests.Load() != 0 {
				t.Fatalf("preloaded signal sent %d mutation requests, want 0", requests.Load())
			}
			if stdout.Len() != 0 {
				t.Fatalf("signal produced JSON output: %s", stdout)
			}
		})
	}
}

func TestWaitDispatchPrefersBufferedCompletion(t *testing.T) {
	want := usage("invalid_input", "finished")
	for range 100 {
		signals := make(chan os.Signal, 1)
		done := make(chan error, 1)
		signals <- os.Interrupt
		done <- want
		sig, signaled, got := waitDispatch(signals, done)
		if got != want || sig != nil || signaled {
			t.Fatalf("waitDispatch = (%v, %v, %v), want completed result", got, sig, signaled)
		}
	}
}

func TestRunSignalAfterMutationTransmissionReportsOutcomeUnknown(t *testing.T) {
	for _, test := range []struct {
		name  string
		path  string
		code  string
		args  func(string, string, string) []string
		stdin string
		piped bool
	}{
		{
			name: "create", path: "/api/secrets", code: "create_outcome_unknown", stdin: "create me",
			args: func(server, phrase, _ string) []string {
				return []string{"create", "--server", server, "--json", "--passphrase-file", phrase}
			},
		},
		{
			name: "reveal", path: "/api/secrets/" + contractID + "/reveal", code: "claim_outcome_unknown",
			args: func(server, phrase, _ string) []string {
				return []string{"reveal", "--json", "--passphrase-file", phrase, server + "/s/" + contractID}
			},
		},
		{
			name: "burn", path: "/api/secrets/" + contractID + "/burn", code: "revoke_outcome_unknown",
			args: func(server, _, token string) []string {
				return []string{"burn", "--server", server, "--json", "--token-file", token, contractID}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			received := make(chan struct{})
			requestPath := make(chan string, 1)
			handlerDone := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
			defer releaseHandler()
			var receivedOnce, handlerDoneOnce sync.Once
			srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				receivedOnce.Do(func() {
					requestPath <- r.URL.Path
					close(received)
				})
				select {
				case <-release:
				case <-r.Context().Done():
				}
				handlerDoneOnce.Do(func() { close(handlerDone) })
			}))
			defer srv.Close()

			token := credentialFile(t, "token", contractToken+"\n")
			signals := make(chan os.Signal)
			e, stdout, stderr := contractEnv(test.args(srv.URL, phraseFile(t), token), test.stdin)
			e.StdinPiped = test.piped
			e.Signals = signals
			exitC := make(chan int, 1)
			go func() { exitC <- Run(e) }()

			waitSignalEvent(t, received, "mutation request")
			select {
			case signals <- os.Interrupt:
			case code := <-exitC:
				releaseHandler()
				t.Fatalf("Run returned %d before the signal; stdout=%s stderr=%s", code, stdout, stderr)
			case <-time.After(signalTestTimeout):
				releaseHandler()
				t.Fatal("Run did not receive the signal")
			}

			code := waitSignalExit(t, exitC)
			releaseHandler()
			waitSignalEvent(t, handlerDone, "request handler completion")
			if code != 9 {
				t.Fatalf("exit=%d, want 9; stdout=%s stderr=%s", code, stdout, stderr)
			}
			if requests.Load() != 1 {
				t.Fatalf("requests=%d, want exactly 1", requests.Load())
			}
			if got := <-requestPath; got != test.path {
				t.Fatalf("request path=%q, want %q", got, test.path)
			}
			var result errorResult
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("error JSON: %v; stdout=%s", err, stdout)
			}
			if result.Status != "error" || result.Code != test.code || result.Server != srv.URL {
				t.Fatalf("error JSON=%s, want code %q and server %q", stdout, test.code, srv.URL)
			}
		})
	}
}

func TestRunSignalWaitsForCreateHandoff(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Write([]byte(`{"id":"` + contractID + `","mgmt_token":"` + contractToken + `","ttl":60}`))
	}))
	defer srv.Close()

	w := newSignalGateWriter()
	signals := make(chan os.Signal)
	e, _, _ := contractEnv([]string{
		"create", "--server", srv.URL, "--json", "--passphrase-file", phraseFile(t),
	}, "create handoff")
	e.Stdout = w
	e.Stderr = io.Discard
	e.Signals = signals
	exitC := make(chan int, 1)
	go func() { exitC <- Run(e) }()

	w.waitStarted(t)
	select {
	case signals <- syscall.SIGTERM:
	case code := <-exitC:
		w.releaseWrite()
		t.Fatalf("Run returned %d before the handoff write received a signal", code)
	case <-time.After(signalTestTimeout):
		w.releaseWrite()
		t.Fatal("Run did not receive the signal")
	}

	var earlyExit *int
	select {
	case code := <-exitC:
		earlyExit = &code
	case <-time.After(100 * time.Millisecond):
	}
	w.releaseWrite()
	w.waitFinished(t)
	code := 0
	if earlyExit != nil {
		code = *earlyExit
	} else {
		code = waitSignalExit(t, exitC)
	}
	if earlyExit != nil {
		t.Fatalf("Run returned %d before the required create handoff completed", code)
	}
	if code != 143 {
		t.Fatalf("exit=%d, want 143", code)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests=%d, want 1", requests.Load())
	}
	var result createResult
	if err := json.Unmarshal(w.bytes(), &result); err != nil {
		t.Fatalf("create receipt JSON: %v; stdout=%s", err, w.bytes())
	}
	if result.Status != "created" || result.Link != srv.URL+"/s/"+contractID || result.MgmtToken != contractToken {
		t.Fatalf("create receipt was not completed: %s", w.bytes())
	}
}

func TestRunSubstantiveHandoffFailureBeatsSignal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"` + contractID + `","mgmt_token":"` + contractToken + `","ttl":60}`))
	}))
	defer srv.Close()

	w := newSignalGateWriter(io.ErrClosedPipe)
	signals := make(chan os.Signal)
	e, _, stderr := contractEnv([]string{
		"create", "--server", srv.URL, "--passphrase-file", phraseFile(t),
	}, "create handoff")
	e.Stdout = w
	e.Signals = signals
	exitC := make(chan int, 1)
	go func() { exitC <- Run(e) }()

	w.waitStarted(t)
	select {
	case signals <- syscall.SIGTERM:
	case code := <-exitC:
		w.releaseWrite()
		t.Fatalf("Run returned %d before the handoff received a signal", code)
	case <-time.After(signalTestTimeout):
		w.releaseWrite()
		t.Fatal("Run did not receive the signal")
	}
	select {
	case code := <-exitC:
		w.releaseWrite()
		t.Fatalf("Run returned %d before the handoff failure completed", code)
	case <-time.After(100 * time.Millisecond):
	}
	w.releaseWrite()
	if code := waitSignalExit(t, exitC); code != 3 {
		t.Fatalf("exit=%d, want authoritative local failure 3; stderr=%s", code, stderr)
	}
}

func TestReadBoundedCancellationClosesActivePipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := readBounded(ctx, r, 64)
		result <- err
	}()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v, want context.Canceled", err)
		}
	case <-time.After(signalTestTimeout):
		t.Fatal("readBounded did not close and join the active pipe read")
	}
}

func TestMapDecryptErrorPreservesRetryCommandErrors(t *testing.T) {
	for _, test := range []struct {
		name  string
		err   error
		exit  int
		code  string
		cause error
	}{
		{name: "interruption", err: interrupted(context.Canceled), exit: 130, cause: context.Canceled},
		{name: "local I/O", err: localCause("cannot read retry phrase", io.ErrClosedPipe), exit: 3, code: "local_io_failed", cause: io.ErrClosedPipe},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := mapDecryptError(test.err, "https://example.com")
			var command commandError
			if !errors.As(err, &command) || command.exit != test.exit || command.code != test.code ||
				command.server != "https://example.com" || !errors.Is(err, test.cause) {
				t.Fatalf("mapped error = %#v, want server-attached command error %#v", err, test)
			}
		})
	}
}

func waitSignalEvent(t *testing.T, event <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-event:
	case <-time.After(signalTestTimeout):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func waitSignalExit(t *testing.T, exitC <-chan int) int {
	t.Helper()
	select {
	case code := <-exitC:
		return code
	case <-time.After(signalTestTimeout):
		t.Fatal("timed out waiting for Run to return")
		return 0
	}
}

type signalGateWriter struct {
	started, release, finished           chan struct{}
	startOnce, releaseOnce, finishedOnce sync.Once
	mu                                   sync.Mutex
	buf                                  bytes.Buffer
	err                                  error
}

func newSignalGateWriter(writeErr ...error) *signalGateWriter {
	var err error
	if len(writeErr) > 0 {
		err = writeErr[0]
	}
	return &signalGateWriter{
		started: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{}), err: err,
	}
}

func (w *signalGateWriter) Write(p []byte) (int, error) {
	w.startOnce.Do(func() { close(w.started) })
	<-w.release
	if w.err != nil {
		w.finishedOnce.Do(func() { close(w.finished) })
		return 0, w.err
	}
	w.mu.Lock()
	n, err := w.buf.Write(p)
	w.mu.Unlock()
	w.finishedOnce.Do(func() { close(w.finished) })
	return n, err
}

func (w *signalGateWriter) waitStarted(t *testing.T) { waitSignalEvent(t, w.started, "handoff write") }
func (w *signalGateWriter) waitFinished(t *testing.T) {
	waitSignalEvent(t, w.finished, "handoff completion")
}
func (w *signalGateWriter) releaseWrite() { w.releaseOnce.Do(func() { close(w.release) }) }
func (w *signalGateWriter) bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buf.Bytes()...)
}
