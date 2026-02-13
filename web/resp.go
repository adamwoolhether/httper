package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// RespondJSON to an HTTP request, setting the status code and body if any.
func RespondJSON(w http.ResponseWriter, r *http.Request, statusCode int, data any) error {
	SetStatusCode(r.Context(), statusCode)

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
// for web.Error (uses its code), FieldErrors (responds 422), or wraps the
// message in {"error": "..."} with the given status code.
func RespondError(w http.ResponseWriter, r *http.Request, statusCode int, err error) error {
	var webErr Error
	if errors.As(err, &webErr) {
		return RespondJSON(w, r, webErr.Code, webErr)
	}

	var fieldErrs FieldErrors
	if errors.As(err, &fieldErrs) {
		return RespondJSON(w, r, http.StatusUnprocessableEntity, fieldErrs)
	}

	return RespondJSON(w, r, statusCode, struct {
		Error string `json:"error"`
	}{Error: err.Error()})
}

// Redirect issues an HTTP redirect to the given URL. The status code
// must be in the 3xx range or an error is returned.
func Redirect(w http.ResponseWriter, r *http.Request, url string, code int) error {
	if code < 300 || code > 399 { // code < 300 || code > 399
		return fmt.Errorf("invalid redirect code: %d", code)
	}

	SetStatusCode(r.Context(), code)

	http.Redirect(w, r, url, code)

	return nil
}
