package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/burnerpad/burnerpad-cli/envelope"
	"github.com/burnerpad/burnerpad-cli/internal/api"
	"github.com/burnerpad/burnerpad-cli/internal/secret"
)

type createResult struct {
	Status    string `json:"status"`
	Server    string `json:"server"`
	Link      string `json:"link"`
	Phrase    string `json:"phrase"`
	MgmtToken string `json:"mgmt_token"`
	TTL       int64  `json:"ttl"`
}

type revealResult struct {
	Status    string `json:"status"`
	Server    string `json:"server"`
	Plaintext string `json:"plaintext"`
}

type burnResult struct {
	Status string `json:"status"`
	Server string `json:"server"`
}

type decryptResult struct {
	Status    string `json:"status"`
	Plaintext string `json:"plaintext"`
}

type errorResult struct {
	Status     string `json:"status"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Server     string `json:"server,omitempty"`
	RetryAfter *int64 `json:"retry_after,omitempty"`
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func (a *application) discloseServer(server string) error {
	if _, err := fmt.Fprintf(a.env.Stderr, "burnerpad: server: %s\n", server); err != nil {
		return attachServer(local("cannot disclose the selected server"), server)
	}
	return nil
}

func (a *application) warn(message string) {
	fmt.Fprintln(a.env.Stderr, "burnerpad: warning: "+message)
}

func (a *application) warnBeforeNetwork(message string) error {
	if _, err := fmt.Fprintln(a.env.Stderr, "burnerpad: warning: "+message); err != nil {
		return local("cannot write a required security warning")
	}
	return nil
}

func mapAPIError(err error, server string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, api.ErrUnavailable) {
		return commandError{exit: 4, code: "secret_unavailable", message: "the secret is unavailable", server: server, cause: err}
	}
	var unknown api.OutcomeUnknownError
	if errors.As(err, &unknown) {
		code := map[string]string{"create": "create_outcome_unknown", "claim": "claim_outcome_unknown", "revoke": "revoke_outcome_unknown"}[unknown.Operation]
		if code == "" {
			code = "internal"
		}
		return commandError{exit: 9, code: code, message: "the request may have changed server state, but its outcome could not be confirmed", server: server, cause: err}
	}
	var limited api.RateLimitedError
	if errors.As(err, &limited) {
		return commandError{exit: 7, code: "rate_limited", message: "the server rate-limited the request", server: server, retryAfter: limited.RetryAfter, cause: err}
	}
	var temporary api.TemporaryError
	if errors.As(err, &temporary) {
		code, message := "service_unavailable", "the server is temporarily unavailable"
		if temporary.Cause != nil {
			code, message = "network_unavailable", "the server could not be reached"
		}
		return commandError{exit: 7, code: code, message: message, server: server, retryAfter: temporary.RetryAfter, cause: err}
	}
	var rejected api.RejectedError
	if errors.As(err, &rejected) {
		return commandError{exit: 6, code: "server_rejected", message: "the server rejected the request", server: server, cause: err}
	}
	var protocol api.ProtocolError
	if errors.As(err, &protocol) {
		return commandError{exit: 8, code: "invalid_server_response", message: "the server returned an invalid response", server: server, cause: err}
	}
	return commandError{exit: 10, code: "internal", message: "unexpected internal failure", server: server, cause: err}
}

func mapDecryptError(err error, server string) error {
	var command commandError
	if errors.As(err, &command) {
		command.server = server
		return command
	}
	if errors.Is(err, envelope.ErrAuthFail) {
		return commandError{exit: 5, code: "passphrase_failed", message: "the passphrase did not open the secret", server: server}
	}
	if errors.Is(err, envelope.ErrUnsupportedSuite) || errors.Is(err, envelope.ErrTruncated) {
		return commandError{exit: 8, code: "unsupported_secret", message: "the ciphertext is not a supported Burnerpad passphrase secret", server: server}
	}
	return commandError{exit: 10, code: "internal", message: "unexpected internal failure", server: server}
}

func attachServer(err error, server string) error {
	var command commandError
	if errors.As(err, &command) {
		command.server = server
		return command
	}
	return err
}

type reservedFile struct {
	file    *os.File
	path    string
	written bool
}

func (r *reservedFile) discardUnlessWritten() {
	if r != nil && !r.written {
		r.discard()
	}
}

func reserve(path string) (*reservedFile, error) {
	if path == "" || path == "-" {
		return nil, usage("invalid_option", "an output file must be a named path")
	}
	f, err := secret.CreateExclusive(path)
	if err != nil {
		return nil, local("cannot create the requested output file exclusively")
	}
	return &reservedFile{file: f, path: path}, nil
}

func (r *reservedFile) write(data []byte) error {
	if _, err := r.file.Write(data); err != nil {
		return local("cannot write the requested output file")
	}
	if err := r.file.Close(); err != nil {
		return local("cannot finish the requested output file")
	}
	r.file = nil
	return nil
}

func (r *reservedFile) discard() {
	if r == nil {
		return
	}
	if r.file != nil {
		_ = r.file.Close()
		r.file = nil
	}
	_ = os.Remove(r.path)
}
