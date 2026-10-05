package middleware

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/adamwoolhether/httper/web/mux"
)

// PanicError is the error Panics returns for a recovered panic.
type PanicError struct {
	Value any
	Stack []byte
}

// Error implements the error interface.
func (e *PanicError) Error() string {
	return fmt.Sprintf("PANIC [%v] TRACE[%s]", e.Value, e.Stack)
}

// Panics recovers from panics if they occur and returns them as a *PanicError.
// Each non-nil onPanic hook runs in order with that error before Panics returns,
// so a hook can report the panic, for example to an error tracker.
// It re-panics http.ErrAbortHandler without running the hooks, so the server
// aborts the response.
func Panics(onPanic ...func(ctx context.Context, err error)) mux.Middleware {
	m := func(handler mux.Handler) mux.Handler {
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) (err error) {
			defer func() {
				if rec := recover(); rec != nil {
					if rec == http.ErrAbortHandler {
						panic(rec)
					}

					err = &PanicError{Value: rec, Stack: debug.Stack()}
					notify(ctx, err, onPanic)
				}
			}()

			return handler(ctx, w, r)
		}
		return h
	}
	return m
}

func notify(ctx context.Context, err error, hooks []func(ctx context.Context, err error)) {
	for _, hook := range hooks {
		if hook != nil {
			hook(ctx, err)
		}
	}
}
