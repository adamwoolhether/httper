package web

import (
	"context"
	"encoding/json"
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if _, err = w.Write(jsonData); err != nil {
		return err
	}

	return nil
}

// RespondError writes a structured JSON error response using the
// status code and message from the given *errs.Error.
func RespondError(ctx context.Context, w http.ResponseWriter, err *errs.Error) error {
	return RespondJSON(ctx, w, err.Code, err)
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
