package middleware

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/errs"
	"github.com/adamwoolhether/httper/web/mux"
)

// DefaultAllowHeaders is the default set of headers permitted in
// cross-origin requests when no custom list is provided to CORS.

// CORS middleware for handling CORS settings.
// If `*` is given, all origins will be accepted.
// Sensivle default headers are set, and can be optionally
// overridden with the variadic allowedHeaders parameter.
func CORS(allowedOrigins []string, allowedHeaders ...string) mux.Middleware {
	defaultHeaders := []string{
		"Authorization",
		"Content-Type",
		"Accept",
		"X-Requested-With",
		"Cache-Control",
	}

	if len(allowedHeaders) == 0 {
		allowedHeaders = defaultHeaders
	}

	originAllowed := CheckOriginFunc(allowedOrigins)
	headers := strings.Join(allowedHeaders, ", ")

	m := func(handler mux.Handler) mux.Handler {
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			origin := r.Header.Get("origin")
			if origin == "" { // Ignore the mw if no Origin header.
				return handler(ctx, w, r)
			}

			if originAllowed(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS, PUT, POST, PATCH, DELETE")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.Header().Set("Access-Control-Allow-Headers", headers)
			} else {
				return web.RespondError(ctx, w, errs.New(http.StatusForbidden, fmt.Errorf("CORS origin[%s] not allowed", origin)))
			}

			if r.Method == http.MethodOptions {
				return web.RespondJSON(ctx, w, http.StatusNoContent, nil)
			}

			return handler(ctx, w, r)
		}
		return h
	}
	return m
}

// CheckOriginFunc loads the list of allowed origins, and returns a func that determines
// if the given origin is valid against the allowable list.
func CheckOriginFunc(allowedOrigins []string) func(string) bool {
	// wildCardCheckFn is a closure to check the given origin against
	// a list of potential wildcard allowed origins.
	wildCardCheckFn := func(wildcards []string, origin string) bool {
		for _, o := range wildcards {
			matches, err := path.Match(o, origin)
			if matches && err == nil {
				return true
			}
		}

		return false
	}

	// Ensure the given list from config is actually an array
	// in case the user gives a comma-separated string instead of an array of strings.
	separated := make([]string, 0)
	for _, o := range allowedOrigins {
		separated = append(separated, strings.Split(o, ",")...)
	}

	allowed := make(map[string]bool)
	wildCardOrigins := make([]string, 0)

	// Collect non-wildcard origins in `allowed` map,
	// and wildcard origins on `wildCardOrigins`.
	for _, o := range separated {
		switch {
		case o == "*": // Check for the `allowAll` catchall.
			allowed["*"] = true
		case strings.Contains(o, "*"):
			wildCardOrigins = append(wildCardOrigins, o)
		default:
			allowed[o] = true
		}
	}
	allowAll := allowed["*"]

	return func(origin string) bool {
		return allowAll || allowed[origin] || wildCardCheckFn(wildCardOrigins, origin)
	}
}
