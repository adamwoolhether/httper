package middleware

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/adamwoolhether/httper/web/mux"
)

// Panics recovers from panics if they occur and returns them as errors.
// It re-panics http.ErrAbortHandler so the server aborts the response.
func Panics() mux.Middleware {
	m := func(handler mux.Handler) mux.Handler {
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) (err error) {
			defer func() {
				if rec := recover(); rec != nil {
					if rec == http.ErrAbortHandler {
						panic(rec)
					}

					trace := debug.Stack()
					err = fmt.Errorf("PANIC [%v] TRACE[%s]", rec, string(trace))
				}
			}()

			return handler(ctx, w, r)
		}
		return h
	}
	return m
}
