package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/adamwoolhether/httper/web/mux"
)

// CSRF uses the standard library CrossOriginProtection to prevent CSRF attacks.
func CSRF(logger *slog.Logger, allowedOrigins ...string) mux.Middleware {
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(errHandler(logger))
	for _, origin := range allowedOrigins {
		if err := cop.AddTrustedOrigin(origin); err != nil {
			panic(err)
		}
	}

	m := func(handler mux.Handler) mux.Handler {
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			var err error

			std := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx = r.Context()

				err = handler(ctx, w, r)
			})

			cop.Handler(std).ServeHTTP(w, r.WithContext(ctx))

			return err
		}

		return h
	}

	return m
}

func errHandler(logger *slog.Logger) http.HandlerFunc {
	f := func(w http.ResponseWriter, r *http.Request) {
		mux.SetStatusCode(r.Context(), http.StatusForbidden)

		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)

		logger.Warn("csrf middleware", "error", "cross origin protection check failed")
	}

	return f
}
