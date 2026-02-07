package client

import (
	"errors"
	"fmt"
)

var (
	ErrUnexpectedStatusCode = errors.New("unexpected status code")
	ErrAuthenticationFailed = errors.New("authentication failed")
)

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
