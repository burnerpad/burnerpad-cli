package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/burnerpad/burnerpad-cli/envelope"
)

const testID = "0123456789ABCDEFGHJKMNPQRS"
const testToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func TestCurrentContractCreateRevealAndBurn(t *testing.T) {
	phrase := []byte("aardvark abandoned abbreviate abdomen abhorrence abiding abnormal")
	blob := envelope.EncryptPassphrase(phrase, []byte("hello"))
	encoded := string(envelope.EncodeToBytes(blob))

	var step atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch step.Add(1) {
		case 1:
			if r.Method != http.MethodPost || r.URL.Path != "/api/secrets" {
				t.Errorf("create request = %s %s", r.Method, r.URL.Path)
			}
			var body struct {
				Blob string `json:"blob"`
				TTL  *int64 `json:"ttl"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode create: %v", err)
			}
			if body.Blob != encoded || body.TTL == nil || *body.TTL != 90 {
				t.Errorf("create body = %+v", body)
			}
			w.Write([]byte(`{"id":"` + testID + `","mgmt_token":"` + testToken + `","ttl":60,"future":true}`))
		case 2:
			if r.Method != http.MethodPost || r.URL.Path != "/api/secrets/"+testID+"/reveal" {
				t.Errorf("reveal request = %s %s", r.Method, r.URL.Path)
			}
			var body map[string]any
			if r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 0 {
				t.Errorf("reveal did not send an empty JSON object")
			}
			w.Write([]byte(`{"blob":"` + encoded + `","future":true}`))
		case 3:
			if r.Method != http.MethodPost || r.URL.Path != "/api/secrets/"+testID+"/burn" {
				t.Errorf("burn request = %s %s", r.Method, r.URL.Path)
			}
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["mgmt_token"] != testToken {
				t.Errorf("burn body = %+v", body)
			}
			w.Write([]byte(`{"status":"burned","future":true}`))
		default:
			t.Errorf("unexpected extra request")
		}
	}))
	defer srv.Close()

	client, err := New(Config{BaseURL: srv.URL, Timeout: time.Second, UserAgent: "burnerpad-cli/test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ttl := int64(90)
	created, err := client.Create(context.Background(), blob, &ttl)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != testID || created.MgmtToken != testToken || created.TTL != 60 {
		t.Fatalf("Create = %+v", created)
	}
	gotBlob, err := client.Reveal(context.Background(), testID)
	if err != nil {
		t.Fatalf("Reveal: %v", err)
	}
	if string(gotBlob) != string(blob) {
		t.Fatal("Reveal returned a different blob")
	}
	if err := client.Burn(context.Background(), testID, testToken); err != nil {
		t.Fatalf("Burn: %v", err)
	}
	if step.Load() != 3 {
		t.Fatalf("requests = %d, want 3", step.Load())
	}
}

func TestCurrentContractUnavailableIsGeneric(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not_found"}`))
	}))
	defer srv.Close()
	client, err := New(Config{BaseURL: srv.URL, Timeout: time.Second, UserAgent: "burnerpad-cli/test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Reveal(context.Background(), testID); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Reveal error = %v, want ErrUnavailable", err)
	}
	if err := client.Burn(context.Background(), testID, testToken); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Burn error = %v, want ErrUnavailable", err)
	}
}

func TestCurrentContractMutationStatusClassification(t *testing.T) {
	for _, operation := range []string{"create", "claim", "revoke"} {
		for status := 100; status <= 599; status++ {
			if status == http.StatusOK {
				continue // operation-specific success-body validation owns HTTP 200
			}
			t.Run(operation+"/"+strconv.Itoa(status), func(t *testing.T) {
				resp := &http.Response{
					StatusCode: status,
					Header:     http.Header{"Retry-After": []string{"17"}},
					Body:       http.NoBody,
				}
				err := classifyStatus(resp, operation)
				want := "unknown"
				switch status {
				case http.StatusNotFound:
					if operation != "create" {
						want = "unavailable"
					}
				case http.StatusTooManyRequests:
					want = "rate limited"
				case http.StatusServiceUnavailable:
					want = "temporary"
				case http.StatusBadRequest, http.StatusRequestEntityTooLarge:
					if operation == "create" {
						want = "rejected"
					}
				}
				switch want {
				case "unavailable":
					if !errors.Is(err, ErrUnavailable) {
						t.Fatalf("HTTP %d error = %T %v, want unavailable", status, err, err)
					}
				case "rate limited":
					var limited RateLimitedError
					if !errors.As(err, &limited) || limited.RetryAfter == nil || *limited.RetryAfter != 17 {
						t.Fatalf("HTTP %d error = %T %v, want rate limit with Retry-After 17", status, err, err)
					}
				case "temporary":
					var temporary TemporaryError
					if !errors.As(err, &temporary) || temporary.RetryAfter == nil || *temporary.RetryAfter != 17 {
						t.Fatalf("HTTP %d error = %T %v, want temporary failure with Retry-After 17", status, err, err)
					}
				case "rejected":
					var rejected RejectedError
					if !errors.As(err, &rejected) || rejected.Status != status {
						t.Fatalf("HTTP %d error = %T %v, want definitive rejection", status, err, err)
					}
				case "unknown":
					var unknown OutcomeUnknownError
					if !errors.As(err, &unknown) || unknown.Operation != operation {
						t.Fatalf("HTTP %d error = %T %v, want %s outcome unknown", status, err, err, operation)
					}
				}
			})
		}
	}
}

func TestCurrentContractInvalidSuccessBodiesAreOutcomeUnknown(t *testing.T) {
	for _, test := range []struct {
		operation string
		invoke    func(*Client) error
	}{
		{operation: "create", invoke: func(c *Client) error {
			_, err := c.Create(context.Background(), []byte{2, 1}, nil)
			return err
		}},
		{operation: "claim", invoke: func(c *Client) error {
			_, err := c.Reveal(context.Background(), testID)
			return err
		}},
		{operation: "revoke", invoke: func(c *Client) error {
			return c.Burn(context.Background(), testID, testToken)
		}},
	} {
		t.Run(test.operation, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			client, err := New(Config{BaseURL: srv.URL, Timeout: time.Second, UserAgent: "test"})
			if err != nil {
				t.Fatal(err)
			}
			err = test.invoke(client)
			var unknown OutcomeUnknownError
			if !errors.As(err, &unknown) || unknown.Operation != test.operation {
				t.Fatalf("error = %T %v, want %s outcome unknown", err, err, test.operation)
			}
		})
	}
}

func TestCurrentContractNeverRetriesTimedOutMutation(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte(`{"blob":"AA"}`))
	}))
	defer srv.Close()
	client, err := New(Config{BaseURL: srv.URL, Timeout: 20 * time.Millisecond, UserAgent: "burnerpad-cli/test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = client.Reveal(context.Background(), testID)
	var unknown OutcomeUnknownError
	if !errors.As(err, &unknown) || unknown.Operation != "claim" {
		t.Fatalf("Reveal error = %T %v, want claim OutcomeUnknownError", err, err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
}

func TestCurrentContractCreateAndBurnAlsoNeverRetry(t *testing.T) {
	for _, tc := range []struct {
		name, operation string
		invoke          func(*Client) error
	}{
		{"create", "create", func(client *Client) error {
			_, err := client.Create(context.Background(), []byte{2, 1}, nil)
			return err
		}},
		{"burn", "revoke", func(client *Client) error { return client.Burn(context.Background(), testID, testToken) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				time.Sleep(100 * time.Millisecond)
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			client, err := New(Config{BaseURL: srv.URL, Timeout: 20 * time.Millisecond, UserAgent: "test"})
			if err != nil {
				t.Fatal(err)
			}
			err = tc.invoke(client)
			var unknown OutcomeUnknownError
			if !errors.As(err, &unknown) || unknown.Operation != tc.operation {
				t.Fatalf("error=%T %v", err, err)
			}
			if requests.Load() != 1 {
				t.Fatalf("requests=%d", requests.Load())
			}
		})
	}
}

func TestCurrentContractTransportPolicy(t *testing.T) {
	if _, err := New(Config{BaseURL: "http://example.com", Timeout: time.Second, UserAgent: "test"}); err == nil {
		t.Fatal("remote plain HTTP was accepted")
	}
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer srv.Close()
	client, err := New(Config{BaseURL: srv.URL, Timeout: time.Second, UserAgent: "test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Reveal(context.Background(), testID)
	var unknown OutcomeUnknownError
	if !errors.As(err, &unknown) || unknown.Operation != "claim" {
		t.Fatalf("redirect error=%T %v, want claim outcome unknown", err, err)
	}
	if requests.Load() != 1 {
		t.Fatalf("redirect requests=%d", requests.Load())
	}
}
