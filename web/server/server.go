package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// Server wraps an [http.Server] with signal-driven graceful shutdown.
type Server struct {
	srv             *http.Server
	shutdownTimeout time.Duration
	logger          *slog.Logger
	shutdownFuncs   []shutdownFunc
	tlsCertFile     string
	tlsKeyFile      string
}

// New creates a Server for the given handler. A default host of ":8080",
// sensible timeouts, and the default slog logger are used unless
// overridden via options.
func New(handler http.Handler, opts ...Option) *Server {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	srv := o.srv
	if srv == nil {
		srv = &http.Server{
			Addr:         ":8080",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		}
	}

	srv.Handler = handler

	if o.host != "" {
		srv.Addr = o.host
	}
	if o.readTimeout != 0 {
		srv.ReadTimeout = o.readTimeout
	}
	if o.writeTimeout != 0 {
		srv.WriteTimeout = o.writeTimeout
	}
	if o.idleTimeout != 0 {
		srv.IdleTimeout = o.idleTimeout
	}

	s := Server{
		srv:             srv,
		shutdownTimeout: 20 * time.Second,
		logger:          slog.Default(),
	}

	if o.shutdownTimeout != 0 {
		s.shutdownTimeout = o.shutdownTimeout
	}
	if o.logger != nil {
		s.logger = o.logger
	}
	if len(o.shutdownFuncs) > 0 {
		s.shutdownFuncs = o.shutdownFuncs
	}
	if o.tlsCertFile != "" {
		s.tlsCertFile = o.tlsCertFile
		s.tlsKeyFile = o.tlsKeyFile
	}

	return &s
}

// Run starts the HTTP server and blocks until a SIGINT or SIGTERM signal
// is received, then performs a graceful shutdown. It returns nil on clean
// shutdown or an error if the server fails to start or shut down.
func (s *Server) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErrs := make(chan error, 1)
	go func() {
		s.logger.Info("server started", "addr", s.srv.Addr)

		if s.tlsCertFile != "" {
			serverErrs <- s.srv.ListenAndServeTLS(s.tlsCertFile, s.tlsKeyFile)
		} else {
			serverErrs <- s.srv.ListenAndServe()
		}
	}()

	select {
	case err := <-serverErrs:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", err)
		}

		return nil

	case <-ctx.Done():
		stop()
		s.logger.Info("shutdown signal received")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()

		if err := s.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}

		s.logger.Info("shutdown complete")

		return nil
	}
}

// Shutdown gracefully shuts down the server. It first runs any registered
// shutdown functions in order, then drains in-flight requests. Callers
// should set a deadline on ctx to bound how long shutdown may take.
func (s *Server) Shutdown(ctx context.Context) error {
	for _, fn := range s.shutdownFuncs {
		if err := fn(ctx); err != nil {
			s.logger.Error("shutdown func", "error", err)
		}
	}

	if err := s.srv.Shutdown(ctx); err != nil {
		s.srv.Close()
		return fmt.Errorf("server didn't stop gracefully: %w", err)
	}

	return nil
}
