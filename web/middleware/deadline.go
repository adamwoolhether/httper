package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/adamwoolhether/httper/web/mux"
)

// Deadline sets the read and write deadlines of each request to now plus d,
// so it bounds a route that outlasts the server timeouts. It returns an error,
// and does not call the handler, when the ResponseWriter does not support
// deadlines.
func Deadline(d time.Duration) mux.Middleware {
	m := func(handler mux.Handler) mux.Handler {
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			deadline := time.Now().Add(d)
			rc := http.NewResponseController(w)

			if err := rc.SetReadDeadline(deadline); err != nil {
				return fmt.Errorf("set read deadline: %w", err)
			}
			if err := rc.SetWriteDeadline(deadline); err != nil {
				return fmt.Errorf("set write deadline: %w", err)
			}

			return handler(ctx, w, r)
		}

		return h
	}

	return m
}
