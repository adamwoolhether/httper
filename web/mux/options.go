package mux

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"reflect"
	"runtime"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

type Option func(*options)

// options represents optional parameters.
type options struct {
	staticFS   Handler
	staticPath string
	tracer     trace.Tracer
	logger     *slog.Logger
	globalMW   []Middleware
	mw         []Middleware
}

type ordered struct {
	priority int
	global   bool
	fn       Middleware
}

// WithMiddleware auto-categorizes the given middleware by function name,
// assigns priorities, and splits them into global vs route-level stacks.
// Known global middleware (CORS, CSRF) runs on every request via ServeHTTP.
// Known route middleware (Logger, Errors, Panics) and any custom middleware
// run per-route in priority order.
func WithMiddleware(mw ...Middleware) Option {
	mwOrdered := make([]ordered, 0, len(mw))
	globalOrdered := make([]ordered, 0)

	for _, m := range mw {
		switch name(m) {
		case "CORS":
			globalOrdered = append(globalOrdered, ordered{priority: 1, global: true, fn: m})
		case "CSRF":
			globalOrdered = append(globalOrdered, ordered{priority: 2, global: true, fn: m})
		case "Logger":
			mwOrdered = append(mwOrdered, ordered{priority: 3, global: false, fn: m})
		case "Errors":
			mwOrdered = append(mwOrdered, ordered{priority: 4, global: false, fn: m})
		case "Panics":
			mwOrdered = append(mwOrdered, ordered{priority: 100, global: false, fn: m})
		default:
			mwOrdered = append(mwOrdered, ordered{priority: 5, global: false, fn: m})
		}
	}

	slices.SortStableFunc(globalOrdered, func(a, b ordered) int {
		return a.priority - b.priority
	})
	slices.SortStableFunc(mwOrdered, func(a, b ordered) int {
		return a.priority - b.priority
	})

	globalSorted := make([]Middleware, len(globalOrdered))
	for i, v := range globalOrdered {
		globalSorted[i] = v.fn
	}

	mwSorted := make([]Middleware, len(mwOrdered))
	for i, v := range mwOrdered {
		mwSorted[i] = v.fn
	}

	return Option(func(opts *options) {
		opts.globalMW = globalSorted
		opts.mw = mwSorted
	})
}

// WithTracer injects the given tracer into the App.
func WithTracer(tracer trace.Tracer) Option {
	return Option(func(opts *options) {
		opts.tracer = tracer
	})
}

// WithLogger sets the logger used by the App for internal errors.
func WithLogger(log *slog.Logger) Option {
	return Option(func(opts *options) {
		opts.logger = log
	})
}

// WithStaticFS serves static files from fsys under the given URL path prefix.
// The prefix is stripped before looking up files in fsys.
func WithStaticFS(fsys fs.FS, pathPrefix string) Option {
	return Option(func(opts *options) {
		fsHandler := http.StripPrefix(pathPrefix, http.FileServer(http.FS(fsys)))
		opts.staticFS = Adapt(fsHandler)
		opts.staticPath = pathPrefix
	})
}

// Adapt converts a standard http.Handler into a web Handler, enabling
// registration of third-party or stdlib handlers on the App.
func Adapt(h http.Handler) Handler {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		h.ServeHTTP(w, r)
		return nil
	}
}

func name(mw Middleware) string {
	fnName := runtime.FuncForPC(reflect.ValueOf(mw).Pointer()).Name()

	// Strip package path: ".../web/middleware.CORS.func1" → "middleware.CORS.func1"
	if i := strings.LastIndex(fnName, "/"); i >= 0 {
		fnName = fnName[i+1:]
	}

	// Split by "." → ["middleware", "CORS", "func1"]
	// Index 1 is the enclosing function name.
	parts := strings.Split(fnName, ".")
	if len(parts) >= 2 {
		return parts[1]
	}

	return fnName
}
