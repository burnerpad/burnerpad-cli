package envelope

// Error is a constant, comparable error (works with == and errors.Is). These
// four values are the ONLY errors any exported function of this package can
// return. Their strings are exactly the canonical reject reasons of SPEC §6 —
// vector vocabulary, not prose; never reword them. They carry no dynamic
// context by construction, so a reject can never drag phrase/blob/plaintext
// material into an error chain, log line, or stderr message.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrUnsupportedSuite Error = "reject_unsupported_suite"
	ErrTruncated        Error = "reject_truncated"
	ErrBadEncoding      Error = "reject_bad_encoding"
	ErrAuthFail         Error = "auth_fail"
)
