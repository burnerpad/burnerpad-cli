package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sync/atomic"
)

func (c *Client) do(ctx context.Context, method, path string, body []byte) (*http.Response, bool, error) {
	u := url.URL{Scheme: c.base.Scheme, Host: c.base.Host, Path: path}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	var wrote atomic.Bool
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{WroteHeaders: func() { wrote.Store(true) }})
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.ua)
	resp, err := c.hc.Do(req)
	return resp, wrote.Load(), err
}
