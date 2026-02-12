package mux

import "go.opentelemetry.io/otel/trace"

// Options represents optional parameters.
type Options struct {
	staticFS Handler
	tracer   trace.Tracer
}

// WithTracer injects the given tracer into the Mux.
func WithTracer(tracer trace.Tracer) func(opts *Options) {
	return func(opts *Options) {
		opts.tracer = tracer
	}
}

// WithStaticFS enables an assets FS for our server to use.
func WithStaticFS(h Handler) func(opts *Options) {
	return func(opts *Options) {
		opts.staticFS = h
	}
}
