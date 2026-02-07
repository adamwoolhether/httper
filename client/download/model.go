package download

import (
	"errors"
	"fmt"
)

var (
	ErrContentLengthMismatch = errors.New("content length mismatch")
	ErrChecksumMismatch      = errors.New("checksum mismatch")
)

type Error struct {
	Detail string
	Err    error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%v: %s", e.Err, e.Detail)
}

func (e *Error) Unwrap() error {
	return e.Err
}
