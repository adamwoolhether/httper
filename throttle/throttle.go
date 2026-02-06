package throttle

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// throttle is an http.RoundTripper, using the time/rate token
// bucket limiter to restrict outbound calls.
type throttle struct {
	limiter *rate.Limiter
	rps     int
	burst   int
	next    http.RoundTripper
	logFn   func() *slog.Logger
}

var (
	ErrMustNotBeZero = errors.New("must be greater than zero")
	ErrWaitingFailed = errors.New("limiter waiting failed")
	ErrContextEnded  = errors.New("throttle context ended")
)

// NewRoundTripper returns an http.RoundTripper that throttles outbound requests
// using a token bucket rate limiter. logFn lazily resolves the logger at request
// time, making option ordering irrelevant. A nil-returning logFn skips the calls
// to *Limiter.Allow().
func NewRoundTripper(rps, burst int, logFn func() *slog.Logger, next http.RoundTripper) (http.RoundTripper, error) {
	if rps <= 0 || burst <= 0 {
		return nil, fmt.Errorf("rps[%d] and burst[%d] %w", rps, burst, ErrMustNotBeZero)
	}

	t := &throttle{
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
		rps:     rps,
		burst:   burst,
		next:    next,
		logFn:   logFn,
	}

	return t, nil
}

func (t *throttle) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.limiter == nil {
		return t.next.RoundTrip(r)
	}

	ctx := r.Context()

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w early: %w", ErrContextEnded, err)
	}

	var waited time.Duration
	logger := t.logFn()
	if logger != nil && !t.limiter.Allow() {
		logger.Info("throttle tokens exhausted", "rate", t.rps, "burst", t.burst, "path", r.URL.Path)

		defer func() {
			logger.Info("throttle wait complete", "waited", waited.String(), "rate", t.rps, "burst", t.burst)
		}()
	}

	start := time.Now()

	err := t.limiter.Wait(ctx)
	waited = time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWaitingFailed, err)
	}

	if err := ctx.Err(); err != nil { // Check context hasn't expired again.
		return nil, fmt.Errorf("%w post-wait: %w", ErrContextEnded, err)
	}

	return t.next.RoundTrip(r)
}
