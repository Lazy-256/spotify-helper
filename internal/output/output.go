// Package output defines the JSON output and error contract shared by every
// command (see the output-contract spec).
package output

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
)

// Code is a stable, machine-readable error identifier.
type Code string

const (
	CodeInternal         Code = "internal"
	CodeAPIError         Code = "api_error"
	CodeUsage            Code = "usage"
	CodeNotConfigured    Code = "not_configured"
	CodeNotAuthenticated Code = "not_authenticated"
	CodeNoActiveDevice   Code = "no_active_device"
	CodeDeviceNotFound   Code = "device_not_found"
	CodeAmbiguousDevice  Code = "ambiguous_device"
	CodeRateLimited      Code = "rate_limited"
	CodePremiumRequired  Code = "premium_required"
	CodeNetworkError     Code = "network_error"
)

// ExitCode returns the process exit code for the error code.
func (c Code) ExitCode() int {
	switch c {
	case CodeUsage:
		return 2
	case CodeNotConfigured, CodeNotAuthenticated:
		return 3
	case CodeNoActiveDevice, CodeDeviceNotFound, CodeAmbiguousDevice:
		return 4
	case CodeRateLimited:
		return 5
	case CodePremiumRequired:
		return 6
	case CodeNetworkError:
		return 7
	default: // internal, api_error and anything unknown
		return 1
	}
}

// Error is the one error type that leaves the program.
type Error struct {
	Code    Code
	Message string
	// RetryAfter is the number of seconds to wait; set for CodeRateLimited.
	RetryAfter int
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

// Errorf builds an Error with a formatted message.
func Errorf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Usage builds a usage error.
func Usage(format string, args ...any) *Error {
	return Errorf(CodeUsage, format, args...)
}

// RateLimited builds a rate_limited error; seconds is raised to at least 1.
func RateLimited(seconds int) *Error {
	if seconds < 1 {
		seconds = 1
	}
	return &Error{
		Code:       CodeRateLimited,
		Message:    fmt.Sprintf("rate limited by Spotify; retry in %d s", seconds),
		RetryAfter: seconds,
	}
}

// Network turns a transport failure into a network_error. The request URL is
// dropped from the message: it is noise for the caller and may carry data.
func Network(err error) *Error {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		err = ue.Err
	}
	msg := err.Error()
	if ne, ok := errors.AsType[net.Error](err); (ok && ne.Timeout()) || errors.Is(err, context.DeadlineExceeded) {
		msg = "request timed out"
	}
	return Errorf(CodeNetworkError, "%s", msg)
}

// Wrap converts any error into an *Error; unknown errors become internal.
func Wrap(err error) *Error {
	if oe, ok := errors.AsType[*Error](err); ok {
		return oe
	}
	return &Error{Code: CodeInternal, Message: err.Error()}
}

// Encode renders v as a single-line JSON document followed by a newline.
// HTML characters are not escaped so names such as "A&B" stay readable.
func Encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteJSON writes v as one JSON document plus newline.
func WriteJSON(w io.Writer, v any) error {
	b, err := Encode(v)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// OK is the payload of commands that have nothing to report.
type OK struct {
	OK bool `json:"ok"`
}

// WriteOK writes {"ok":true}.
func WriteOK(w io.Writer) error { return WriteJSON(w, OK{OK: true}) }

type errorBody struct {
	Code       Code   `json:"code"`
	Message    string `json:"message"`
	RetryAfter int    `json:"retry_after,omitempty"`
}

// WriteError writes the failure document {"error":{…}} to w.
func WriteError(w io.Writer, e *Error) {
	b, err := Encode(struct {
		Error errorBody `json:"error"`
	}{errorBody{Code: e.Code, Message: e.Message, RetryAfter: e.RetryAfter}})
	if err != nil {
		// Encoding a plain struct cannot realistically fail; keep the contract
		// even then.
		b = []byte(`{"error":{"code":"internal","message":"cannot encode error"}}` + "\n")
	}
	_, _ = w.Write(b)
}
