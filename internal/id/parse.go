package id

import (
	"net/url"
	"strings"
)

type Target struct {
	Origin string
	ID     string
}

// ParseShareURL accepts only a full current /s/<id> share URL.
func ParseShareURL(raw string) (Target, error) {
	if raw == "" || !strings.Contains(raw, "://") || strings.Contains(raw, "#") {
		return Target{}, ErrBadTarget
	}
	return parseURL(raw)
}

// ParseBurnTarget accepts a full share URL or a bare current identifier.
func ParseBurnTarget(raw string) (Target, error) {
	if raw == "" || strings.Contains(raw, "#") || strings.HasPrefix(raw, "/") {
		return Target{}, ErrBadTarget
	}
	if strings.Contains(raw, "://") {
		return parseURL(raw)
	}
	secretID, err := Normalize(raw)
	if err != nil {
		return Target{}, err
	}
	return Target{ID: secretID}, nil
}

func parseURL(raw string) (Target, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Target{}, ErrBadTarget
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil ||
		u.Opaque != "" || u.ForceQuery || u.RawQuery != "" || u.Fragment != "" {
		return Target{}, ErrBadTarget
	}
	idPart, ok := strings.CutPrefix(u.EscapedPath(), "/s/")
	if !ok || strings.Contains(idPart, "/") {
		return Target{}, ErrBadTarget
	}
	secretID, err := Normalize(idPart)
	if err != nil {
		return Target{}, err
	}
	return Target{Origin: strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host), ID: secretID}, nil
}
