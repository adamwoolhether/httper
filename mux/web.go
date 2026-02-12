// Package mux exposes some helpers to simplify middleware and route handling.
package mux

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Mux contains the web app's main mux routing logic.
type Mux struct {
	mux      *http.ServeMux
	globalMW []Middleware
	mw       []Middleware
	group    string
	tracer   trace.Tracer
}

// Handler is a http.Handler that returns an error.
type Handler func(w http.ResponseWriter, r *http.Request) error

// Middleware defines a signature to chain Handler together.
type Middleware func(handler Handler) Handler

func New(options ...func(*Options)) *Mux {
	var opts Options
	for _, opt := range options {
		opt(&opts)
	}

	mux := http.NewServeMux()

	app := &Mux{
		mux:    mux,
		tracer: noop.NewTracerProvider().Tracer("no-op tracer"),
	}

	if opts.tracer != nil {
		app.tracer = opts.tracer
	}

	if opts.staticFS != nil {
		app.handleNoMiddleware(http.MethodGet, "", "/static/", opts.staticFS)
	}

	return app
}

func (m *Mux) Group() *Mux {
	return &Mux{
		mux:    m.mux,
		mw:     slices.Clone(m.mw),
		tracer: m.tracer,
	}
}

func (m *Mux) Mount(subRoute string) *Mux {
	return &Mux{
		mux:    m.mux,
		mw:     slices.Clone(m.mw),
		group:  subRoute,
		tracer: m.tracer,
	}
}

func (m *Mux) Use(mw ...Middleware) {
	m.mw = append(m.mw, mw...)
}

func (m *Mux) ApplyGlobalMW(mw ...Middleware) {
	m.globalMW = append(m.globalMW, mw...)
}

func (m *Mux) Get(path string, fn Handler, mw ...Middleware) {
	m.handle(http.MethodGet, m.group, path, fn, mw...)
}

func (m *Mux) Post(path string, fn Handler, mw ...Middleware) {
	m.handle(http.MethodPost, m.group, path, fn, mw...)
}

func (m *Mux) Put(path string, fn Handler, mw ...Middleware) {
	m.handle(http.MethodPut, m.group, path, fn, mw...)
}

func (m *Mux) Patch(path string, fn Handler, mw ...Middleware) {
	m.handle(http.MethodPatch, m.group, path, fn, mw...)
}

func (m *Mux) Delete(path string, fn Handler, mw ...Middleware) {
	m.handle(http.MethodDelete, m.group, path, fn, mw...)
}

// ServeHTTP implements http.Handler, wrapping global middleware before serving the request.
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serveHTTP := func(w http.ResponseWriter, r *http.Request) error {
		m.mux.ServeHTTP(w, r)
		return nil
	}
	wrapped := wrap(m.globalMW, serveHTTP)

	_ = wrapped(w, r)
}

func (m *Mux) handle(method, group, path string, handler Handler, mw ...Middleware) {
	handler = wrap(mw, handler)
	handler = wrap(m.mw, handler)

	h := func(w http.ResponseWriter, r *http.Request) {
		ctx, span := m.startSpan(w, r)
		defer span.End()

		v := BaseValues{
			TraceID: uuid.NewString(),
			Now:     time.Now().UTC(),
			Tracer:  m.tracer,
		}
		r = r.WithContext(setValues(ctx, &v))

		_ = handler(w, r)
	}

	finalPath := path
	if group != "" {
		finalPath = fmt.Sprintf("%s%s", group, path)
	}

	pattern := fmt.Sprintf("%s %s", method, finalPath)

	m.mux.HandleFunc(pattern, h)
} // handleNoMiddleware runs the handleware without any middleware.

func (m *Mux) handleNoMiddleware(method, group, path string, handler Handler) {
	h := func(w http.ResponseWriter, r *http.Request) {
		_ = handler(w, r)
	}

	finalPath := path
	if group != "" {
		finalPath = fmt.Sprintf("/%s%s", group, path)
	}

	pattern := fmt.Sprintf("%s %s", method, finalPath)

	m.mux.HandleFunc(pattern, h)
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

// startSpan initializes the request by adding a span and writing
// otel-related info into the response writer for the response.
func (m *Mux) startSpan(w http.ResponseWriter, r *http.Request) (context.Context, trace.Span) {
	ctx, span := m.tracer.Start(r.Context(), "mux.handler")
	span.SetAttributes(attribute.String("path", r.RequestURI))

	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(w.Header()))

	return ctx, span
}
