package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/adamwoolhether/httper/web/mux"
)

// CSRF uses the standard library CrossOriginProtection to prevent CSRF attacks.
// Each trusted origin must be an exact scheme://host[:port] value, such as
// "https://app.example.com:8443", with no path, query, or trailing slash.
// Wildcards are not supported: an entry such as "https://*.example.com" is
// accepted but matches no origin. CSRF panics if a trusted origin is invalid.
func CSRF(allowedOrigins ...string) mux.Middleware {
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(errHandler())
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

func errHandler() http.HandlerFunc {
	f := func(w http.ResponseWriter, r *http.Request) {
		mux.SetStatusCode(r.Context(), http.StatusForbidden)

		http.Error(w, fmt.Errorf("csrf: %s", http.StatusText(http.StatusForbidden)).Error(), http.StatusForbidden)
	}

	return f
}
