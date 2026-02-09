package download

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
)

// WorkFunc is the signature for async work.
type WorkFunc func(ctx context.Context) error

// Adder matches the client.DownloadAsync func signature.
// Allows us to inject into the result.
type Adder func(*http.Request, int, string, ...Option) (*Result, error)

// Queue manages a batch of concurrent async downloads.
type Queue struct {
	wg       sync.WaitGroup
	mu       sync.Mutex
	sem      chan struct{}
	shutdown atomic.Bool
	errs     []error
}

// NewQueue creates a Queue with the given concurrency limit and
// returns both the Queue (for Wait/Shutdown) and an Option (for DownloadAsync).
// If maxConcurrent <= 0, concurrency is unlimited.
func NewQueue(maxConcurrent int) *Queue {
	q := &Queue{}
	if maxConcurrent > 0 {
		q.sem = make(chan struct{}, maxConcurrent)
	}
	return q
}

// Wait blocks until all downloads in the group complete.
// Returns all errors joined via errors.Join.
func (g *Queue) Wait() error {
	g.wg.Wait()

	g.mu.Lock()
	defer g.mu.Unlock()

	return errors.Join(g.errs...)
}

// Shutdown prevents new work from executing in this group.
func (g *Queue) Shutdown() {
	g.shutdown.Store(true)
}

// Start launches fn in a new goroutine managed by the group
// and returns a Result for tracking the individual download.
func (g *Queue) Start(ctx context.Context, fn WorkFunc, adder Adder) *Result {
	ctx, cancel := context.WithCancel(ctx)
	r := &Result{
		adder:  adder,
		done:   make(chan struct{}),
		cancel: cancel,
		group:  g,
	}

	g.wg.Add(1)
	go func() {
		defer func() {
			cancel()
			close(r.done)
			g.wg.Done()
		}()

		if g.sem != nil {
			select {
			case g.sem <- struct{}{}:
				defer func() {
					<-g.sem
				}()
			case <-ctx.Done():
				r.err = ctx.Err()
				g.recordErr(r.err)
				return
			}
		}

		if g.shutdown.Load() {
			r.err = ErrGroupShutdown
			g.recordErr(r.err)
			return
		}

		r.err = fn(ctx)
		if r.err != nil {
			g.recordErr(r.err)
		}
	}()

	return r
}
