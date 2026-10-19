package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/adamwoolhether/httper/web/mux"
)

// Deadline sets the write deadline of each request, and the read deadline of
// a request with a body, to now plus d. The deadlines replace the server's
// WriteTimeout and ReadTimeout for that request, so a route can run longer or
// shorter than them. It returns an error, and does not call the handler, when
// the ResponseWriter does not support deadlines. Deadline panics if d is not
// positive, because that deadline fails every read and write of the route.
//
// Put Deadline before any middleware that reads the request body. Once the
// body is consumed, net/http reads the connection in the background, and a
// read deadline set after that cancels the connection's context.
func Deadline(d time.Duration) mux.Middleware {
	if d <= 0 {
		panic(fmt.Sprintf("middleware.Deadline: d must be positive, got %v", d))
	}

	m := func(handler mux.Handler) mux.Handler {
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			deadline := time.Now().Add(d)
			rc := http.NewResponseController(w)

			// net/http reads the connection in the background to detect a closed client,
			// at once when there is no body, and a read deadline there cancels the
			// connection's context.
			if r.ContentLength != 0 {
				if err := rc.SetReadDeadline(deadline); err != nil {
					return fmt.Errorf("set read deadline: %w", err)
				}
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
