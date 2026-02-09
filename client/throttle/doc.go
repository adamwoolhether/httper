// Package throttle provides an [http.RoundTripper] that rate-limits
// outbound HTTP requests using a token-bucket algorithm from
// [golang.org/x/time/rate].
//
// # Usage
//
// Wrap an existing transport with [NewRoundTripper]:
//
//	rt, err := throttle.NewRoundTripper(
//		10,  // requests per second
//		5,   // burst capacity
//		func() *slog.Logger { return slog.Default() },
//		http.DefaultTransport,
//	)
//	httpClient := &http.Client{Transport: rt}
//
// When the rate limit is exceeded, outbound requests block until a
// token becomes available or the request context is cancelled.
package throttle
