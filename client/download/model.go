package download

import (
	"context"
	"errors"
	"fmt"
	"io"
)

var (
	// ErrContentLengthMismatch indicates the number of bytes received did not match the Content-Length header.
	ErrContentLengthMismatch = errors.New("content length mismatch")
	// ErrChecksumMismatch indicates the downloaded file's checksum did not match the expected value.
	ErrChecksumMismatch = errors.New("checksum mismatch")
	// ErrDownloadCancelled indicates the download was cancelled via context cancellation.
	ErrDownloadCancelled = errors.New("download cancelled")
	// ErrGroupShutdown indicates the [Queue] was shut down before this download could start.
	ErrGroupShutdown = errors.New("group is shut down")
)

// Error wraps a sentinel error with additional detail about what went wrong.
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
