package mux

import (
	"encoding/json"
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if _, err = w.Write(jsonData); err != nil {
		return err
	}

	return nil
}

func Redirect(w http.ResponseWriter, r *http.Request, url string, code int) error {
	if code < 300 || code > 399 { // code < 300 || code > 399
		return fmt.Errorf("invalid redirect code: %d", code)
	}

	SetStatusCode(r.Context(), code)

	http.Redirect(w, r, url, code)

	return nil
}
