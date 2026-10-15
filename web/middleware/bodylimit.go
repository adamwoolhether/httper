package middleware

import (
	"context"
	"net/http"

	"github.com/adamwoolhether/httper/web/mux"
)

// BodyLimit limits each request body to n bytes. A read past n returns an
// error that wraps *http.MaxBytesError, and Errors answers it with 413.
func BodyLimit(n int64) mux.Middleware {
	m := func(handler mux.Handler) mux.Handler {
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			r.Body = http.MaxBytesReader(w, r.Body, n)

			return handler(ctx, w, r)
		}

		return h
	}

	return m
}
