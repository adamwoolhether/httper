package mux

import (
	"go.opentelemetry.io/otel/trace"
)

// options represents optional parameters.
type options struct {
	staticFS Handler
	tracer   trace.Tracer
	globalMW []Middleware
}

func WithCORS(allowsOrigins ...string) func(opts *options) {
	return func(opts *options) {
		opts.globalMW = append(opts.globalMW, cors(allowsOrigins...))
	}
}

// WithGlobalMW applies a set of middleware that is applied to all handlers.
func WithGlobalMW(mw ...Middleware) func(opts *options) {
	return func(opts *options) {
		opts.globalMW = mw
	}
}

// WithTracer injects the given tracer into the Mux.
func WithTracer(tracer trace.Tracer) func(opts *options) {
	return func(opts *options) {
		opts.tracer = tracer
	}
}

// WithStaticFS enables an assets FS for our server to use.
func WithStaticFS(h Handler) func(opts *options) {
	return func(opts *options) {
		opts.staticFS = h
	}
}

func Static() Handler {
	// f := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	// 	subFS, err := fs.Sub(publicFS, "assets")
	// 	if err != nil {
	// 		return fmt.Errorf("couldn't load assets: %w", err)
	// 	}
	//
	// 	h := http.StripPrefix(web.StaticPath, http.FileServer(http.FS(subFS)))
	// 	h.ServeHTTP(w, r)
	//
	// 	return nil
	// }
	//
	// return f
	return nil
}
