package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/adamwoolhether/httper/web/errs"
	"github.com/adamwoolhether/httper/web/mux"
)

// RespondJSON to an HTTP request, setting the status code and body if any.
func RespondJSON(ctx context.Context, w http.ResponseWriter, statusCode int, data any) error {
	mux.SetStatusCode(ctx, statusCode)

	if statusCode == http.StatusNoContent {
		w.WriteHeader(statusCode)
		return nil
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.WriteHeader(statusCode)

	if _, err = w.Write(jsonData); err != nil {
		return err
	}

	return nil
}

// RespondError writes a structured JSON error response. It inspects the error
// for *errs.Error (uses its code), errs.FieldErrors (responds 422), or wraps the
// message in {"error": "..."} with the given status code.
func RespondError(ctx context.Context, w http.ResponseWriter, statusCode int, err error) error {
	var appErr *errs.Error
	if errors.As(err, &appErr) {
		return RespondJSON(ctx, w, appErr.Code, appErr)
	}

	var fieldErrs errs.FieldErrors
	if errors.As(err, &fieldErrs) {
		return RespondJSON(ctx, w, http.StatusUnprocessableEntity, fieldErrs)
	}

	return RespondJSON(ctx, w, statusCode, struct {
		Error string `json:"error"`
	}{Error: err.Error()})
}

// Redirect issues an HTTP redirect to the given URL. The status code
// must be in the 3xx range or an error is returned.
func Redirect(w http.ResponseWriter, r *http.Request, url string, code int) error {
	if code < 300 || code > 399 {
		return fmt.Errorf("invalid redirect code: %d", code)
	}

	mux.SetStatusCode(r.Context(), code)

	http.Redirect(w, r, url, code)

	return nil
}
