package mux

import (
	"fmt"
	"net/http"
	"path"
	"strings"
)

// cors middleware for handling cors settings.
// If `*` is given, all origins will be accepted.
// We explicitly call web.Respond for errors here,
// as this middleware is wrapped globally and errors
// may potentially not be seen by errors middleware.
func cors(allowedOrigins ...string) Middleware {
	originAllowed := CheckOriginFunc(allowedOrigins)

	m := func(handler Handler) Handler {
		h := func(w http.ResponseWriter, r *http.Request) error {
			origin := r.Header.Get("origin")
			if origin == "" { // Ignore the mw if no Origin header.
				return handler(w, r)
			}

			if originAllowed(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS, PUT, POST, PATCH, DELETE")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.Header().Set("Access-Control-Allow-Headers",
					"Content-Type, Depth, User-Agent, X-File-Size, X-Requested-With, If-Modified-Since, X-File-Name, "+
						"Cache-Control, Authorization, Accept, Accept-Encoding, Accept-Language, "+
						"Access-Control-Request-Headers, Access-Control-Request-Method, Connection, Host, Origin, "+
						"X-User-Agent, App-Version")
			} else {
				return RespondJSON(w, r, http.StatusForbidden, fmt.Errorf("cors origin[%s] not allowed", origin))
			}

			if r.Method == http.MethodOptions {
				return RespondJSON(w, r, http.StatusNoContent, nil)
			}

			return handler(w, r)
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

	// Ensure the given list from config is actually an array.
	// Needed because the comma-seperated list from Terraform is
	// being interpreted as a single string.
	seperated := make([]string, 0)
	for _, o := range allowedOrigins {
		seperated = append(seperated, strings.Split(o, ",")...)
	}

	allowed := make(map[string]bool)
	wildCardOrigins := make([]string, 0)

	// collected non-wildcard origins in `allowed` map,
	// and wildcard origins on `wilCardOrigins`.
	for _, o := range seperated {
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
