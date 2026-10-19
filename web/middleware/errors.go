package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"

	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/errs"
	"github.com/adamwoolhether/httper/web/mux"
)

// Errors handles errors coming out of the call chain.
// An error that wraps *http.MaxBytesError, from BodyLimit or web.DecodeLimit,
// gets a 413 response, even when it also wraps an *errs.Error or
// errs.FieldErrors.
// Each onUnknown hook runs with an error that is not an *errs.Error,
// errs.FieldErrors, or *http.MaxBytesError, for example to report it to an
// error tracker. A hook that panics is not recovered.
func Errors(log *slog.Logger, onUnknown ...func(ctx context.Context, err error)) mux.Middleware {
	m := func(handler mux.Handler) mux.Handler {
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			err := handler(ctx, w, r)
			if err == nil {
				return nil
			}

			maxErr, tooLarge := errors.AsType[*http.MaxBytesError](err)

			if fieldErr, ok := errors.AsType[errs.FieldErrors](err); ok && !tooLarge {
				return web.RespondJSON(ctx, w, http.StatusUnprocessableEntity, fieldErr)
			}

			appErr, ok := errors.AsType[*errs.Error](err)
			switch {
			case tooLarge:
				appErr = errs.New(http.StatusRequestEntityTooLarge, fmt.Errorf("request body exceeds %d bytes", maxErr.Limit))
			case !ok: // to catch errs that may have escaped, obscure them from public view.
				runHooks(ctx, err, onUnknown)
				appErr = errs.NewInternal(err)
			}

			reqLog := log.With("trace_id", mux.GetValues(ctx).TraceID)
			reqLog.Error(err.Error(), "source_err_file", path.Base(appErr.FileName), "source_err_func", path.Base(appErr.FuncName))

			if appErr.InnerErr { // after logging, obscure the internal error from public view.
				public := *appErr
				public.Message = http.StatusText(appErr.Code)
				appErr = &public
			}

			return web.RespondJSON(ctx, w, appErr.Code, appErr)
		}

		return h
	}

	return m
}
