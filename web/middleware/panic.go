package middleware

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/adamwoolhether/httper/web/errs"
	"github.com/adamwoolhether/httper/web/mux"
)

// Panics recovers from panics if they occur and returns them as internal errors.
// Each onPanic hook runs with the panic error before Panics returns, for
// example to report it to an error tracker. A hook that panics is not recovered.
// It re-panics http.ErrAbortHandler so the server aborts the response.
func Panics(onPanic ...func(ctx context.Context, err error)) mux.Middleware {
	m := func(handler mux.Handler) mux.Handler {
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) (err error) {
			defer func() {
				if rec := recover(); rec != nil {
					if rec == http.ErrAbortHandler {
						panic(rec)
					}

					panicErr := fmt.Errorf("PANIC [%v] TRACE[%s]", rec, debug.Stack())
					runHooks(ctx, panicErr, onPanic)
					err = errs.NewInternal(panicErr)
				}
			}()

			return handler(ctx, w, r)
		}
		return h
	}
	return m
}

func runHooks(ctx context.Context, err error, hooks []func(ctx context.Context, err error)) {
	for _, hook := range hooks {
		if hook != nil {
			hook(ctx, err)
		}
	}
}
