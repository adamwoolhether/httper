package throttle

import (
	"errors"
	"log/slog"
	"net/http"

	"golang.org/x/time/rate"
)

var (
	ErrMustNotBeZero = errors.New("must be greater than zero")
	ErrWaitingFailed = errors.New("limiter waiting failed")
	ErrContextEnded  = errors.New("throttle context ended")
)

// Config defines the throttler's
// Requests Per Second and Burst Rate
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
