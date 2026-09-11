package api

import (
	"errors"
	"strconv"
)

var ErrUnavailable = errors.New("secret unavailable")

type OutcomeUnknownError struct{ Operation string }

func (e OutcomeUnknownError) Error() string { return e.Operation + " outcome is unknown" }

type TemporaryError struct {
	Cause      error
	RetryAfter *int64
}

func (TemporaryError) Error() string   { return "service temporarily unavailable" }
func (e TemporaryError) Unwrap() error { return e.Cause }

type RateLimitedError struct{ RetryAfter *int64 }

func (RateLimitedError) Error() string { return "rate limited" }

type RejectedError struct{ Status int }

func (e RejectedError) Error() string {
	return "server rejected request (HTTP " + strconv.Itoa(e.Status) + ")"
}

func mutationError(operation string, wrote bool, cause error) error {
	if wrote {
		return OutcomeUnknownError{Operation: operation}
	}
	return TemporaryError{Cause: cause}
}
