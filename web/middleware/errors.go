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

			var appErr *errs.Error
			if !errors.As(err, &appErr) {
				appErr = errs.NewInternal(err)
			}

			log.Error(err.Error(), "source_err_file", path.Base(appErr.FileName), "source_err_func", path.Base(appErr.FuncName))

			if appErr.InnerErr {
				appErr.Message = "internal server error"
			}

			return web.RespondJSON(ctx, w, appErr.Code, appErr.Message)
		}

		return h
	}

	return m
}
