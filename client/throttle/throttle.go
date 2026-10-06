package throttle

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// NewRoundTripper returns an http.RoundTripper that throttles outbound requests
// using a token bucket rate limiter. logFn lazily resolves the logger at request
// time, making option ordering irrelevant.
func NewRoundTripper(rps, burst int, logFn func() *slog.Logger, next http.RoundTripper) (http.RoundTripper, error) {
	if rps <= 0 || burst <= 0 {
		return nil, fmt.Errorf("rps[%d] and burst[%d] %w", rps, burst, ErrMustNotBeZero)
	}

	return newThrottle(rate.Limit(rps), burst, logFn, next), nil
}

// NewRoundTripperEvery is [NewRoundTripper] at one request per interval, for rates that whole requests per second cannot express.
func NewRoundTripperEvery(interval time.Duration, burst int, logFn func() *slog.Logger, next http.RoundTripper) (http.RoundTripper, error) {
	if interval <= 0 || burst <= 0 {
		return nil, fmt.Errorf("interval[%s] and burst[%d] %w", interval, burst, ErrMustNotBeZero)
	}

	return newThrottle(rate.Every(interval), burst, logFn, next), nil
}

func newThrottle(limit rate.Limit, burst int, logFn func() *slog.Logger, next http.RoundTripper) *throttle {
	return &throttle{
		limiter: rate.NewLimiter(limit, burst),
		burst:   burst,
		next:    next,
		logFn:   logFn,
	}
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
	if logger != nil && t.limiter.Tokens() < 1 {
		rps := float64(t.limiter.Limit())
		logger.Info("throttle tokens exhausted", "rate", rps, "burst", t.burst, "path", r.URL.Path)

		defer func() {
			logger.Info("throttle wait complete", "waited", waited.String(), "rate", rps, "burst", t.burst)
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
