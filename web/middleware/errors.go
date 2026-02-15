package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path"

	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/errs"
	"github.com/adamwoolhether/httper/web/mux"
)

// Errors handles errors coming out of the call chain.
func Errors(log *slog.Logger) mux.Middleware {
	m := func(handler mux.Handler) mux.Handler {
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			err := handler(ctx, w, r)
			if err == nil {
				return nil
			}

			if fieldErr, ok := errors.AsType[errs.FieldErrors](err); ok {
				return web.RespondJSON(ctx, w, http.StatusUnprocessableEntity, fieldErr)
			}

			appErr, ok := errors.AsType[*errs.Error](err)
			if !ok { // to catch errs that may have escaped, obscure them from public view.
				appErr = errs.NewInternal(err)
			}

			reqLog := log.With("trace_id", mux.GetValues(ctx).TraceID)
			reqLog.Error(err.Error(), "source_err_file", path.Base(appErr.FileName), "source_err_func", path.Base(appErr.FuncName))

			if appErr.InnerErr { // after logging, obscure the internal error from public view.
				appErr.Message = http.StatusText(appErr.Code)
			}

			return web.RespondJSON(ctx, w, appErr.Code, appErr)
		}

		return h
	}

	return m
}
