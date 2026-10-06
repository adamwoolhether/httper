package throttle

import (
	"errors"
	"log/slog"
	"net/http"

	"golang.org/x/time/rate"
)

var (
	// ErrMustNotBeZero indicates that the rate or interval and the burst must be positive.
	ErrMustNotBeZero = errors.New("must be greater than zero")
	// ErrWaitingFailed indicates the rate limiter's wait call failed.
	ErrWaitingFailed = errors.New("limiter waiting failed")
	// ErrContextEnded indicates the request context expired before or after the rate-limit wait.
	ErrContextEnded = errors.New("throttle context ended")
)

// throttle is an http.RoundTripper, using the time/rate token
// bucket limiter to restrict outbound calls.
type throttle struct {
	limiter *rate.Limiter
	burst   int
	next    http.RoundTripper
	logFn   func() *slog.Logger
}
