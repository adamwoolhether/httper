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

	if err := t.wait(r); err != nil {
		// The RoundTripper contract makes the transport close the body on errors too; http.Client does not.
		// The wait error is the one to report, so a close error adds nothing.
		if r.Body != nil {
			_ = r.Body.Close()
		}
		return nil, err
	}

	return t.next.RoundTrip(r)
}

func (t *throttle) wait(r *http.Request) error {
	ctx := r.Context()

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w early: %w", ErrContextEnded, err)
	}

	logger := t.logFn()
	exhausted := logger != nil && logger.Enabled(ctx, slog.LevelDebug) && t.limiter.Tokens() < 1
	if exhausted {
		logger.DebugContext(ctx, "throttle tokens exhausted", "rate", float64(t.limiter.Limit()), "burst", t.burst, "path", r.URL.Path)
	}

	start := time.Now()
	if err := t.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("%w: %w", ErrWaitingFailed, err)
	}
	if exhausted {
		logger.DebugContext(ctx, "throttle wait complete", "waited", time.Since(start).String(), "rate", float64(t.limiter.Limit()), "burst", t.burst)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w post-wait: %w", ErrContextEnded, err)
	}

	return nil
}
