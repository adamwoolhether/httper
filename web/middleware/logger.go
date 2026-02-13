package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/adamwoolhether/httper/web/mux"
)

// Logger logs the start and completion of each request, including
// method, path, remote address, status code, and elapsed time.
func Logger(log *slog.Logger) mux.Middleware {
	m := func(handler mux.Handler) mux.Handler {
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			v := mux.GetValues(ctx)

			path := r.URL.Path
			if r.URL.RawQuery != "" {
				path = fmt.Sprintf("%s?%s", path, r.URL.RawQuery)
			}

			log.Info("request started", "method", r.Method, "path", path, "remoteaddr", r.RemoteAddr)

			err := handler(ctx, w, r)

			log.Info("request completed", "method", r.Method, "path", path, "remoteaddr", r.RemoteAddr, "statusCode", v.StatusCode, "since", time.Since(v.Now).String())

			return err
		}

		return h
	}

	return m
}
