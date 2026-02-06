package throttle

import (
	"errors"
	"log/slog"
	"net/http"

	"golang.org/x/time/rate"
)

var (
	// ErrMustNotBeZero indicates that RPS and burst must be positive.
	ErrMustNotBeZero = errors.New("must be greater than zero")
	// ErrWaitingFailed indicates the rate limiter's Wait call failed.
	ErrWaitingFailed = errors.New("limiter waiting failed")
	// ErrContextEnded indicates the request context expired before or after the rate-limit wait.
	ErrContextEnded = errors.New("throttle context ended")
)

// Config defines the throttler's rate-limiting parameters: requests per second (RPS) and burst capacity.
type Config struct {
	RPS   int
	Burst int
}

// throttle is an http.RoundTripper, using the time/rate token
// bucket limiter to restrict outbound calls.
type throttle struct {
	limiter *rate.Limiter
	rps     int
	burst   int
	next    http.RoundTripper
	logFn   func() *slog.Logger
}
