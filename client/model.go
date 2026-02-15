package client

import (
	"errors"
	"fmt"
	"net/http"
)

// maxErrBodySize caps the amount of response body read when
// building an error for an unexpected status code. This prevents
// unbounded memory usage when a large response arrives with a
// wrong status.
const maxErrBodySize = 4 << 10 // 4KB

// execFn represents a func to operate on a response.
type execFn func(response *http.Response) error

var (
	// ErrUnexpectedStatusCode is the sentinel error wrapped by [UnexpectedStatusError].
	ErrUnexpectedStatusCode = errors.New("unexpected status code")
	// ErrAuthFailure is joined with [ErrUnexpectedStatusCode] when the server
	// responds with 401 Unauthorized or 403 Forbidden.
	ErrAuthFailure = errors.New("auth failure")
)

// UnexpectedStatusError is returned when the HTTP response status code
// does not match the expected value.
type UnexpectedStatusError struct {
	StatusCode int
	Body       string
	Err        error
}

func (e *UnexpectedStatusError) Error() string {
	return fmt.Sprintf("%v: %d, body: %s", e.Err, e.StatusCode, e.Body)
}

func (e *UnexpectedStatusError) Unwrap() error {
	return e.Err
}
