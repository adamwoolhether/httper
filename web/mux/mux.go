// Package mux provides helpers for middleware and route handling.
package mux

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// App is the core web application, managing routing and middleware.
type App struct {
	mux      *http.ServeMux
	globalMW []Middleware
	mw       []Middleware
	group    string
	host     string
	logger   *slog.Logger
	tracer   trace.Tracer
}

// Handler is a http.Handler that returns an error.
type Handler func(ctx context.Context, w http.ResponseWriter, r *http.Request) error

// Middleware defines a signature to chain Handler together.
type Middleware func(handler Handler) Handler

// New creates an App with the given options. A no-op tracer and the
// default slog logger are used unless overridden via options.
func New(optFns ...Option) *App {
	var opts options
	for _, opt := range optFns {
		opt(&opts)
	}
	if opts.logger == nil {
		opts.logger = slog.Default()
	}
	if opts.tracer == nil {
		opts.tracer = noop.NewTracerProvider().Tracer("no-op tracer")
	}

	mux := http.NewServeMux()

	app := &App{
		mux:      mux,
		globalMW: opts.globalMW,
		mw:       opts.mw,
		logger:   opts.logger,
		tracer:   opts.tracer,
	}

	if opts.staticFS != nil {
		app.HandleNoMiddleware(http.MethodGet, "", opts.staticPath, opts.staticFS)
	}

	return app
}

// ServeHTTP implements http.Handler, wrapping global middleware before serving the request.
// After the global middleware, it lowercases the request Host so that virtual
// host routes match case-insensitively; route handlers see the lowercased Host.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serveHTTP := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		routed := r
		if host := strings.ToLower(r.Host); host != r.Host {
			routed = r.WithContext(r.Context())
			routed.Host = host
		}

		a.mux.ServeHTTP(w, routed)
		// ServeMux sets Pattern on the request it routes, and outer handlers such as otelhttp read it.
		r.Pattern = routed.Pattern
		return nil
	}
	wrapped := wrap(a.globalMW, serveHTTP)

	if err := wrapped(r.Context(), w, r); err != nil {
		a.logger.Error("mux", "serve http", err)
	}
}

// Group returns a new App that shares the same underlying ServeMux, tracer,
// route prefix, and virtual host but has an independent middleware stack.
func (a *App) Group() *App {
	return &App{
		mux:      a.mux,
		globalMW: a.globalMW,
		mw:       slices.Clone(a.mw),
		group:    a.group,
		host:     a.host,
		logger:   a.logger,
		tracer:   a.tracer,
	}
}

// Mount returns a new App scoped to the given sub-route prefix.
// All routes registered on the returned App are prefixed with the
// current prefix followed by subRoute.
func (a *App) Mount(subRoute string) *App {
	sub := a.Group()
	sub.group = path.Join(a.group, subRoute)
	return sub
}

// VirtualHost returns a new App whose routes match only requests for host,
// such as "api.example.com". Routes without a virtual host match every host,
// and a virtual host route takes precedence over them for its host.
// The host may hold only ASCII letters, digits, '-', and '.', and it is
// lowercased. A scheme, port, path, wildcard, or IPv6 literal never matches
// in ServeMux, which ignores the request port. VirtualHost panics if host is invalid.
func (a *App) VirtualHost(host string) *App {
	invalid := func(c rune) bool {
		return !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '-' || c == '.')
	}
	if host == "" || strings.ContainsFunc(host, invalid) {
		panic(fmt.Sprintf("mux: invalid virtual host %q", host))
	}

	sub := a.Group()
	sub.host = strings.ToLower(host)
	return sub
}

// Use appends the given middleware to the underlying mw stack.
func (a *App) Use(mw ...Middleware) {
	a.mw = append(a.mw, mw...)
}

// Get registers a handler for GET requests at the given path.
func (a *App) Get(path string, fn Handler, mw ...Middleware) {
	a.Handle(http.MethodGet, a.group, path, fn, mw...)
}

// Post registers a handler for POST requests at the given path.
func (a *App) Post(path string, fn Handler, mw ...Middleware) {
	a.Handle(http.MethodPost, a.group, path, fn, mw...)
}

// Put registers a handler for PUT requests at the given path.
func (a *App) Put(path string, fn Handler, mw ...Middleware) {
	a.Handle(http.MethodPut, a.group, path, fn, mw...)
}

// Patch registers a handler for PATCH requests at the given path.
func (a *App) Patch(path string, fn Handler, mw ...Middleware) {
	a.Handle(http.MethodPatch, a.group, path, fn, mw...)
}

// Delete registers a handler for DELETE requests at the given path.
func (a *App) Delete(path string, fn Handler, mw ...Middleware) {
	a.Handle(http.MethodDelete, a.group, path, fn, mw...)
}

func (a *App) Handle(method, group, path string, handler Handler, mw ...Middleware) {
	handler = wrap(mw, handler)
	handler = wrap(a.mw, handler)

	h := func(w http.ResponseWriter, r *http.Request) {
		ctx, span := a.startSpan(w, r)
		defer span.End()

		traceID := span.SpanContext().TraceID().String()
		if !span.SpanContext().TraceID().IsValid() {
			traceID = uuid.New().String()
		}

		v := BaseValues{
			TraceID: traceID,
			Now:     time.Now().UTC(),
			Tracer:  a.tracer,
		}

		r = r.WithContext(setValues(ctx, &v))

		if err := handler(r.Context(), w, r); err != nil {
			a.logger.Error("mux", "handle", err)
		}
	}

	pattern := fmt.Sprintf("%s %s%s", method, a.host, routePath(group, path))

	a.mux.HandleFunc(pattern, h)
}

func (a *App) HandleRaw(method, group, path string, handler http.Handler, mw ...Middleware) {
	a.Handle(method, group, path, adapt(handler), mw...)
}

// HandleNoMiddleware registers a handler without wrapping it in the
// route-level or group-level middleware stack.
func (a *App) HandleNoMiddleware(method, group, path string, handler Handler) {
	h := func(w http.ResponseWriter, r *http.Request) {
		if err := handler(r.Context(), w, r); err != nil {
			a.logger.Error("mux", "handle no mw", err)
		}
	}

	pattern := fmt.Sprintf("%s %s%s", method, a.host, routePath(group, path))

	a.mux.HandleFunc(pattern, h)
}

// routePath joins group and route into a ServeMux path. Either may omit
// its leading slash. An empty route matches only the group path itself.
func routePath(group, route string) string {
	prefix := strings.TrimSuffix(path.Clean("/"+group), "/")

	switch {
	case route != "":
		return prefix + "/" + strings.TrimPrefix(route, "/")
	case prefix == "":
		return "/{$}"
	default:
		return prefix
	}
}

// startSpan initializes the request by adding a span and writing
// otel-related info into the response writer for the response.
func (a *App) startSpan(w http.ResponseWriter, r *http.Request) (context.Context, trace.Span) {
	ctx, span := a.tracer.Start(r.Context(), "mux.handler")
	span.SetAttributes(attribute.String("path", r.RequestURI))

	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(w.Header()))

	return ctx, span
}

// adapt converts a standard http.Handler into a web Handler.
func adapt(h http.Handler) Handler {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		h.ServeHTTP(w, r)
		return nil
	}
}

// wrap middleware around the handler and execute in order given.
func wrap(mw []Middleware, handler Handler) Handler {
	for _, mwFn := range slices.Backward(mw) {
		if mwFn != nil {
			handler = mwFn(handler)
		}
	}

	return handler
}
