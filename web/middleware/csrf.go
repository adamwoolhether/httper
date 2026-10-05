package middleware

import (
	"context"
	"net/http"

	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/errs"
	"github.com/adamwoolhether/httper/web/mux"
)

// CSRF uses the standard library CrossOriginProtection to prevent CSRF attacks.
// It rejects an untrusted cross-origin request with a 403 JSON error.
// Each trusted origin must be an exact scheme://host[:port] value, such as
// "https://app.example.com:8443", with no path, query, or trailing slash.
// Wildcards are not supported: an entry such as "https://*.example.com" is
// accepted but matches no origin. CSRF panics if a trusted origin is invalid.
func CSRF(allowedOrigins ...string) mux.Middleware {
	cop := http.NewCrossOriginProtection()
	for _, origin := range allowedOrigins {
		if err := cop.AddTrustedOrigin(origin); err != nil {
			panic(err)
		}
	}

	m := func(handler mux.Handler) mux.Handler {
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			if err := cop.Check(r); err != nil {
				return web.RespondError(ctx, w, errs.New(http.StatusForbidden, err))
			}

			return handler(ctx, w, r)
		}

		return h
	}

	return m
}
