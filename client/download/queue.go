package download

import (
	"context"
	"errors"
	"net/http"
	"sync"
)

// WorkFunc is the signature for a unit of asynchronous work managed by a [queue].
type WorkFunc func(ctx context.Context) error

// Adder is a callback matching the [Client.DownloadAsync] signature,
// injected into [Result] so that [Result.Add] can enqueue more downloads.
type Adder func(*http.Request, int, string, ...Option) (*Result, error)

// queue manages a batch of concurrent async downloads.
type queue struct {
	wg        sync.WaitGroup
	mu        sync.Mutex
	sem       chan struct{}
	errs      []error
	cancelAll chan struct{}
	closeOnce sync.Once
}

// newQueue creates a queue with the given concurrency limit.
// If maxConcurrent <= 0, concurrency is unlimited.
func newQueue(maxConcurrent int) *queue {
	q := &queue{
		cancelAll: make(chan struct{}),
	}
	if maxConcurrent > 0 {
		q.sem = make(chan struct{}, maxConcurrent)
	}
	return q
}

// Start launches fn in a new goroutine managed by the group
// and returns a Result for tracking the individual download.
func (q *queue) Start(ctx context.Context, fn WorkFunc, adder Adder) *Result {
	ctx, cancel := context.WithCancel(ctx)
	doneCh := make(chan struct{})

	go func() {
		select {
		case <-q.cancelAll:
			cancel()
		case <-doneCh:
		}
	}()

	r := &Result{
		adder:  adder,
		done:   doneCh,
		cancel: cancel,
		group:  q,
	}

	q.wg.Go(func() {
		defer func() {
			cancel()
			close(doneCh)
		}()

		if q.sem != nil {
			select {
			case q.sem <- struct{}{}:
				defer func() { <-q.sem }()
			case <-ctx.Done():
				r.err = ctx.Err()
				q.recordErr(r.err)
				return
			}
		}

		r.err = fn(ctx)
		if r.err != nil {
			q.recordErr(r.err)
		}
	})

	return r
}

// wait blocks until all downloads in the group complete.
// Returns all errors joined via errors.Join.
func (q *queue) wait() error {
	q.wg.Wait()

	q.mu.Lock()
	defer q.mu.Unlock()

	return errors.Join(q.errs...)
}

// doCancelAll closes the cancelAll channel exactly once,
// cancelling every in-flight download in the queue.
func (q *queue) doCancelAll() {
	q.closeOnce.Do(func() { close(q.cancelAll) })
}

// recordErr appends err to the queue's error slice under the mutex.
func (q *queue) recordErr(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.errs = append(q.errs, err)
}
