package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestCurrentContractPostRequestWireFailuresAreOutcomeUnknown(t *testing.T) {
	operations := []struct {
		name, operation, path, successBody string
		invoke                             func(*Client) error
	}{
		{
			name: "create", operation: "create", path: "/api/secrets",
			successBody: `{"id":"` + testID + `","mgmt_token":"` + testToken + `","ttl":60}`,
			invoke: func(client *Client) error {
				_, err := client.Create(context.Background(), []byte{2, 1}, nil)
				return err
			},
		},
		{
			name: "claim", operation: "claim", path: "/api/secrets/" + testID + "/reveal",
			successBody: `{"blob":"AgE"}`,
			invoke: func(client *Client) error {
				_, err := client.Reveal(context.Background(), testID)
				return err
			},
		},
		{
			name: "revoke", operation: "revoke", path: "/api/secrets/" + testID + "/burn",
			successBody: `{"status":"burned"}`,
			invoke: func(client *Client) error {
				return client.Burn(context.Background(), testID, testToken)
			},
		},
	}
	for _, operation := range operations {
		if !json.Valid([]byte(operation.successBody)) {
			t.Fatalf("%s success body is not valid JSON: %q", operation.name, operation.successBody)
		}
	}
	faults := []struct {
		name      string
		truncated bool
	}{
		{name: "connection loss"},
		{name: "truncated declared body", truncated: true},
	}

	for _, operation := range operations {
		for _, fault := range faults {
			t.Run(operation.name+"/"+fault.name, func(t *testing.T) {
				var responseBody []byte
				if fault.truncated {
					responseBody = []byte(operation.successBody)
				}
				server, requests := startPostRequestFaultServer(t, operation.path, responseBody)
				client, err := New(Config{BaseURL: server, Timeout: 2 * time.Second, UserAgent: "burnerpad-cli/test"})
				if err != nil {
					t.Fatalf("New: %v", err)
				}
				t.Cleanup(client.hc.CloseIdleConnections)

				err = operation.invoke(client)
				var unknown OutcomeUnknownError
				if !errors.As(err, &unknown) || unknown.Operation != operation.operation {
					t.Fatalf("error = %T %v, want %s OutcomeUnknownError", err, err, operation.operation)
				}
				if got := requests.Load(); got != 1 {
					t.Fatalf("complete requests = %d, want 1", got)
				}
			})
		}
	}
}

// startPostRequestFaultServer receives complete HTTP/1.1 requests over a real
// loopback TCP connection before either closing without a response or sending
// an otherwise valid response whose declared body is one byte too long.
func startPostRequestFaultServer(t *testing.T, wantPath string, responseBody []byte) (string, *atomic.Int32) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var requests atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- servePostRequestFaults(listener, wantPath, responseBody, &requests)
	}()
	t.Cleanup(func() {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("close fault listener: %v", err)
		}
		if err := <-done; err != nil {
			t.Errorf("fault server: %v", err)
		}
	})

	return "http://" + listener.Addr().String(), &requests
}

func servePostRequestFaults(listener net.Listener, wantPath string, responseBody []byte, requests *atomic.Int32) error {
	for {
		conn, err := listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("accept: %w", err)
		}
		if err := servePostRequestFault(conn, wantPath, responseBody, requests); err != nil {
			return err
		}
	}
}

func servePostRequestFault(conn net.Conn, wantPath string, responseBody []byte, requests *atomic.Int32) error {
	defer conn.Close()
	request, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	if _, err := io.Copy(io.Discard, request.Body); err != nil {
		request.Body.Close()
		return fmt.Errorf("read request body: %w", err)
	}
	if err := request.Body.Close(); err != nil {
		return fmt.Errorf("close request body: %w", err)
	}
	requests.Add(1)
	if request.Method != http.MethodPost || request.URL.Path != wantPath {
		return fmt.Errorf("request = %s %s, want POST %s", request.Method, request.URL.Path, wantPath)
	}
	if responseBody == nil {
		return nil
	}

	response := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: " + strconv.Itoa(len(responseBody)+1) + "\r\n" +
		"Connection: close\r\n\r\n" + string(responseBody)
	if _, err := io.WriteString(conn, response); err != nil {
		return fmt.Errorf("write truncated response: %w", err)
	}
	return nil
}
