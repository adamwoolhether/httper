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
	group  *Queue
}

// Add another download to the same batch.
// It calls the injected Adder and reuses the existing Queue.
// WithBatch cannot be used with this method.
func (r *Result) Add(req *http.Request, expCode int, destPath string, optFns ...Option) (*Result, error) {
	return r.adder(req, expCode, destPath, slices.Concat([]Option{withBatch(r.group)}, optFns)...)
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
func (r *Result) Wait() error { return r.group.Wait() }

// Cancel cancels this download's context.
func (r *Result) Cancel() { r.cancel() }

func (g *Queue) recordErr(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.errs = append(g.errs, err)
}
