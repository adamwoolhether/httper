package web

import (
	"io/fs"
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

// options represents optional parameters.
type options struct {
	staticFS   Handler
	staticPath string
	tracer     trace.Tracer
	globalMW   []Middleware
}

// WithCORS appends CORS middleware to the global middleware stack,
// configured to accept the given origin patterns.
func WithCORS(allowsOrigins ...string) func(opts *options) {
	return func(opts *options) {
		opts.globalMW = append(opts.globalMW, cors(allowsOrigins...))
	}
}

// WithGlobalMW appends the given middleware to the global middleware
// stack that wraps all handlers.
func WithGlobalMW(mw ...Middleware) func(opts *options) {
	return func(opts *options) {
		opts.globalMW = append(opts.globalMW, mw...)
	}
}

// WithTracer injects the given tracer into the App.
func WithTracer(tracer trace.Tracer) func(opts *options) {
	return func(opts *options) {
		opts.tracer = tracer
	}
}

// WithStaticFS serves static files from fsys under the given URL path prefix.
// The prefix is stripped before looking up files in fsys.
func WithStaticFS(fsys fs.FS, pathPrefix string) func(opts *options) {
	return func(opts *options) {
		fsHandler := http.StripPrefix(pathPrefix, http.FileServer(http.FS(fsys)))
		opts.staticFS = Adapt(fsHandler)
		opts.staticPath = pathPrefix
	}
}

// Adapt converts a standard http.Handler into a web Handler, enabling
// registration of third-party or stdlib handlers on the App.
func Adapt(h http.Handler) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		h.ServeHTTP(w, r)
		return nil
	}
}
