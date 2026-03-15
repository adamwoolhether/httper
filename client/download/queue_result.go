package download

import (
	"context"
	"net/http"
	"slices"
)

// Result represents an in-flight or completed async download.
type Result struct {
	adder  Adder
	done   chan struct{}
	err    error
	cancel context.CancelFunc
	group  *queue
}

// Add another download to the same batch.
// It calls the injected Adder and reuses the existing queue.
// WithBatch cannot be used with this method.
//
// Validation errors (empty destPath, conflicting options) are recorded
// in the queue so that [Result.Wait] returns them; the caller does not
// need to check each Add individually.
func (r *Result) Add(req *http.Request, expCode int, destPath string, optFns ...Option) *Result {
	result, err := r.adder(req, expCode, destPath, slices.Concat([]Option{withBatch(r.group)}, optFns)...)
	if err != nil {
		r.group.recordErr(err)
		return &Result{
			adder:  r.adder,
			done:   closedCh,
			err:    err,
			cancel: func() {},
			group:  r.group,
		}
	}
	return result
}

// Done returns a channel that is closed when the specific download completes.
func (r *Result) Done() <-chan struct{} { return r.done }

// Err blocks until this download completes and returns its error.
func (r *Result) Err() error {
	<-r.done
	return r.err
}

// Wait blocks until all downloads in the group complete.
// Returns all errors joined.
func (r *Result) Wait() error {
	return r.group.wait()
}

// Cancel cancels this download's context.
func (r *Result) Cancel() {
	r.cancel()
}

// CancelAll cancels every download in the queue.
func (r *Result) CancelAll() {
	r.group.doCancelAll()
}
