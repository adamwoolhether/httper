package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Option configures a Server.
type Option func(*options)

type options struct {
	srv             *http.Server
	host            string
	readTimeout     time.Duration
	writeTimeout    time.Duration
	idleTimeout     time.Duration
	shutdownTimeout time.Duration
	logger          *slog.Logger
	shutdownFuncs []shutdownFunc
	tlsCertFile   string
	tlsKeyFile    string
}

type shutdownFunc func(ctx context.Context) error

// WithServer injects an existing [http.Server] as the base configuration.
// Any other options applied after this one override the corresponding
// fields on the provided server.
func WithServer(srv *http.Server) Option {
	return Option(func(opts *options) {
		opts.srv = srv
	})
}

// WithHost sets the host address the server listens on. Default is ":8080".
func WithHost(host string) Option {
	return Option(func(opts *options) {
		opts.host = host
	})
}

// WithReadTimeout sets the maximum duration for reading the entire
// request, including the body. Default is 5s.
func WithReadTimeout(d time.Duration) Option {
	return Option(func(opts *options) {
		opts.readTimeout = d
	})
}

// WithWriteTimeout sets the maximum duration before timing out
// writes of the response. Default is 10s.
func WithWriteTimeout(d time.Duration) Option {
	return Option(func(opts *options) {
		opts.writeTimeout = d
	})
}

// WithIdleTimeout sets the maximum amount of time to wait for the
// next request when keep-alives are enabled. Default is 120s.
func WithIdleTimeout(d time.Duration) Option {
	return Option(func(opts *options) {
		opts.idleTimeout = d
	})
}

// WithShutdownTimeout sets the maximum duration [Server.Run] waits for
// in-flight requests to complete after receiving a shutdown signal.
// Default is 20s. Callers of [Server.Shutdown] control the deadline
// via context instead.
func WithShutdownTimeout(d time.Duration) Option {
	return Option(func(opts *options) {
		opts.shutdownTimeout = d
	})
}

// WithLogger sets the logger used for server lifecycle events.
// Default is slog.Default().
func WithLogger(log *slog.Logger) Option {
	return Option(func(opts *options) {
		opts.logger = log
	})
}

// WithShutdownFunc registers a function to call during graceful shutdown,
// before the HTTP server is stopped. Multiple shutdown functions are
// called in the order they were registered.
func WithShutdownFunc(fn func(ctx context.Context) error) Option {
	return Option(func(opts *options) {
		opts.shutdownFuncs = append(opts.shutdownFuncs, fn)
	})
}

// WithTLS configures the server to use TLS with the given certificate
// and key files. When set, the server calls ListenAndServeTLS instead
// of ListenAndServe.
func WithTLS(certFile, keyFile string) Option {
	return Option(func(opts *options) {
		opts.tlsCertFile = certFile
		opts.tlsKeyFile = keyFile
	})
}
