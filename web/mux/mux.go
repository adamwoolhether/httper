// Package mux provides helpers for middleware and route handling.
package mux

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
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
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serveHTTP := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		a.mux.ServeHTTP(w, r)
		return nil
	}
	wrapped := wrap(a.globalMW, serveHTTP)

	if err := wrapped(r.Context(), w, r); err != nil {
		a.logger.Error("mux", "serve http", err)
	}
}

// Group returns a new App that shares the same underlying ServeMux
// and tracer but has an independent middleware stack.
func (a *App) Group() *App {
	return &App{
		mux:      a.mux,
		globalMW: a.globalMW,
		mw:       slices.Clone(a.mw),
		logger:   a.logger,
		tracer:   a.tracer,
	}
}

// Mount returns a new App scoped to the given sub-route prefix.
// All routes registered on the returned App are prefixed with subRoute.
func (a *App) Mount(subRoute string) *App {
	return &App{
		mux:      a.mux,
		globalMW: a.globalMW,
		mw:       slices.Clone(a.mw),
		logger:   a.logger,
		group:    strings.TrimLeft(subRoute, "/"),
		tracer:   a.tracer,
	}
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

	finalPath := path
	if group != "" {
		finalPath = fmt.Sprintf("/%s%s", group, path)
	}

	pattern := fmt.Sprintf("%s %s", method, finalPath)

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

	finalPath := path
	if group != "" {
		finalPath = fmt.Sprintf("/%s%s", group, path)
	}

	pattern := fmt.Sprintf("%s %s", method, finalPath)

	a.mux.HandleFunc(pattern, h)
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
