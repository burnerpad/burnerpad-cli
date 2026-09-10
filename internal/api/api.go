// Package api implements the current burnerpad-lite HTTP contract.
package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/burnerpad/burnerpad-cli/envelope"
	"github.com/burnerpad/burnerpad-cli/internal/id"
)

const bodyCap = 200_000

type Config struct {
	BaseURL   string
	Timeout   time.Duration
	UserAgent string
}

type Client struct {
	hc   *http.Client
	base *url.URL
	ua   string
}

type CreateResult struct {
	ID        string
	MgmtToken string
	TTL       int64
}

func New(cfg Config) (*Client, error) {
	base, err := parseBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	if cfg.Timeout <= 0 {
		return nil, errors.New("api: timeout must be positive")
	}
	if cfg.UserAgent == "" {
		return nil, errors.New("api: user agent is required")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &Client{
		hc: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		base: base,
		ua:   cfg.UserAgent,
	}, nil
}

// ParseBaseURL validates and canonicalizes a configurable server origin.
func ParseBaseURL(raw string) (string, error) {
	u, err := parseBaseURL(raw)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func parseBaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("api: server origin does not parse")
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil ||
		u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("api: server must be an http(s) origin without credentials, path, query, or fragment")
	}
	if u.Scheme == "http" && !loopbackHost(u.Hostname()) {
		return nil, errors.New("api: plain HTTP is allowed only for a loopback server")
	}
	return &url.URL{Scheme: strings.ToLower(u.Scheme), Host: strings.ToLower(u.Host)}, nil
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *Client) Create(ctx context.Context, blob []byte, ttl *int64) (CreateResult, error) {
	request := struct {
		Blob string `json:"blob"`
		TTL  *int64 `json:"ttl,omitempty"`
	}{Blob: string(envelope.EncodeToBytes(blob)), TTL: ttl}
	payload, err := json.Marshal(request)
	if err != nil {
		return CreateResult{}, err
	}
	resp, wrote, err := c.do(ctx, http.MethodPost, "/api/secrets", payload)
	if err != nil {
		return CreateResult{}, mutationError("create", wrote, err)
	}
	if resp.StatusCode != http.StatusOK {
		return CreateResult{}, classifyStatus(resp, "create")
	}
	var result struct {
		ID        *string `json:"id"`
		MgmtToken *string `json:"mgmt_token"`
		TTL       *int64  `json:"ttl"`
	}
	if err := decodeResponse(resp, &result); err != nil {
		return CreateResult{}, OutcomeUnknownError{Operation: "create"}
	}
	if result.ID == nil || result.MgmtToken == nil || result.TTL == nil || *result.TTL <= 0 {
		return CreateResult{}, OutcomeUnknownError{Operation: "create"}
	}
	norm, err := id.Normalize(*result.ID)
	if err != nil || norm != *result.ID || !validToken(*result.MgmtToken) {
		return CreateResult{}, OutcomeUnknownError{Operation: "create"}
	}
	return CreateResult{ID: norm, MgmtToken: *result.MgmtToken, TTL: *result.TTL}, nil
}

func (c *Client) Reveal(ctx context.Context, secretID string) ([]byte, error) {
	if err := checkID(secretID); err != nil {
		return nil, err
	}
	resp, wrote, err := c.do(ctx, http.MethodPost, "/api/secrets/"+secretID+"/reveal", []byte("{}"))
	if err != nil {
		return nil, mutationError("claim", wrote, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, classifyStatus(resp, "claim")
	}
	var result struct {
		Blob *string `json:"blob"`
	}
	if err := decodeResponse(resp, &result); err != nil || result.Blob == nil || *result.Blob == "" {
		return nil, OutcomeUnknownError{Operation: "claim"}
	}
	blob, err := envelope.DecodeCanonical([]byte(*result.Blob))
	if err != nil {
		return nil, OutcomeUnknownError{Operation: "claim"}
	}
	return blob, nil
}

func (c *Client) Burn(ctx context.Context, secretID, token string) error {
	if err := checkID(secretID); err != nil {
		return err
	}
	if !validToken(token) {
		return errors.New("api: invalid management token")
	}
	payload, err := json.Marshal(struct {
		MgmtToken string `json:"mgmt_token"`
	}{token})
	if err != nil {
		return err
	}
	resp, wrote, err := c.do(ctx, http.MethodPost, "/api/secrets/"+secretID+"/burn", payload)
	if err != nil {
		return mutationError("revoke", wrote, err)
	}
	if resp.StatusCode != http.StatusOK {
		return classifyStatus(resp, "revoke")
	}
	var result struct {
		Status *string `json:"status"`
	}
	if err := decodeResponse(resp, &result); err != nil || result.Status == nil || *result.Status != "burned" {
		return OutcomeUnknownError{Operation: "revoke"}
	}
	return nil
}

func checkID(value string) error {
	norm, err := id.Normalize(value)
	if err != nil || norm != value {
		return errors.New("api: invalid canonical id")
	}
	return nil
}

func validToken(token string) bool {
	b, err := envelope.DecodeCanonical([]byte(token))
	if err != nil {
		return false
	}
	defer clear(b)
	return len(b) == 32
}

func decodeResponse(resp *http.Response, dst any) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, bodyCap+1))
	if err != nil {
		return err
	}
	if len(body) > bodyCap {
		return errors.New("response too large")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("trailing response data")
	}
	return nil
}

func classifyStatus(resp *http.Response, operation string) error {
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound:
		if operation == "claim" || operation == "revoke" {
			return ErrUnavailable
		}
		return ProtocolError{Status: resp.StatusCode}
	case http.StatusTooManyRequests:
		return RateLimitedError{RetryAfter: retryAfter(resp)}
	case http.StatusServiceUnavailable:
		return TemporaryError{RetryAfter: retryAfter(resp)}
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge:
		return RejectedError{Status: resp.StatusCode}
	default:
		if resp.StatusCode >= 500 {
			return OutcomeUnknownError{Operation: operation}
		}
		return ProtocolError{Status: resp.StatusCode}
	}
}

func retryAfter(resp *http.Response) *int64 {
	raw := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if raw == "" {
		return nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return nil
	}
	return &n
}
