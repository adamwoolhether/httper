package middleware

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/errs"
	"github.com/adamwoolhether/httper/web/mux"
)

// DefaultAllowHeaders is the default set of headers permitted in
// cross-origin requests when no custom list is provided to CORS.

// CORS sets cross-origin resource sharing headers for allowed origins and
// rejects other origins with 403. CheckOriginFunc defines how origins match.
// If "*" is given, all origins are accepted: the response sets
// Access-Control-Allow-Origin to "*" and does not allow credentials.
// An explicit allowlist reflects the request origin and allows credentials.
// Sensible default headers are set, and can be optionally
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

	allowAll := slices.Contains(splitOrigins(allowedOrigins), "*")
	originAllowed := CheckOriginFunc(allowedOrigins)
	headers := strings.Join(allowedHeaders, ", ")

	m := func(handler mux.Handler) mux.Handler {
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			origin := r.Header.Get("origin")
			if origin == "" { // Ignore the mw if no Origin header.
				return handler(ctx, w, r)
			}

			if !originAllowed(origin) {
				return web.RespondError(ctx, w, errs.New(http.StatusForbidden, fmt.Errorf("CORS origin[%s] not allowed", origin)))
			}

			if allowAll {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS, PUT, POST, PATCH, DELETE")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.Header().Set("Access-Control-Allow-Headers", headers)

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
// Each entry may hold several comma-separated origins; spaces around them are ignored.
// "*" allows every origin. Otherwise one "*" in an entry matches exactly one hostname
// label: "https://*.example.com" matches "https://api.example.com" but not
// "https://example.com", "https://a.b.example.com", or "https://api.example.com:8443".
// An entry with more than one "*" matches nothing.
func CheckOriginFunc(allowedOrigins []string) func(string) bool {
	allowed := make(map[string]bool)
	var wildcards []string

	for _, o := range splitOrigins(allowedOrigins) {
		if o != "*" && strings.Contains(o, "*") {
			wildcards = append(wildcards, o)
			continue
		}
		allowed[o] = true
	}
	allowAll := allowed["*"]

	return func(origin string) bool {
		return allowAll || allowed[origin] || slices.ContainsFunc(wildcards, func(pattern string) bool {
			return matchLabel(pattern, origin)
		})
	}
}

// splitOrigins accepts a comma-separated string in place of an array,
// in case the list comes from config.
func splitOrigins(allowedOrigins []string) []string {
	var origins []string
	for _, entry := range allowedOrigins {
		for o := range strings.SplitSeq(entry, ",") {
			if o = strings.TrimSpace(o); o != "" {
				origins = append(origins, o)
			}
		}
	}

	return origins
}

func matchLabel(pattern, origin string) bool {
	prefix, suffix, _ := strings.Cut(pattern, "*")
	if strings.Contains(suffix, "*") ||
		len(origin) <= len(prefix)+len(suffix) ||
		!strings.HasPrefix(origin, prefix) ||
		!strings.HasSuffix(origin, suffix) {
		return false
	}

	label := origin[len(prefix) : len(origin)-len(suffix)]

	return !strings.ContainsAny(label, ".:/")
}
