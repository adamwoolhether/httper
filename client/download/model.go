package download

import (
	"context"
	"errors"
	"fmt"
	"io"
)

var (
	ErrContentLengthMismatch = errors.New("content length mismatch")
	ErrChecksumMismatch      = errors.New("checksum mismatch")
	ErrDownloadCancelled     = errors.New("download cancelled")
	ErrGroupShutdown         = errors.New("group is shut down")
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

// contextReader wraps an io.Reader with a context.Context
// to allow cancellation of downloads.
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.r.Read(p)
	}
}
