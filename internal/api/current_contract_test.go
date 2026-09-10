package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
	var protocol ProtocolError
	if !errors.As(err, &protocol) {
		t.Fatalf("redirect error=%T %v", err, err)
	}
	if requests.Load() != 1 {
		t.Fatalf("redirect requests=%d", requests.Load())
	}
}
