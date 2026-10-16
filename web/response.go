package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"

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

// RespondFile writes data as a 200 attachment named filename. It sets
// Content-Type, Content-Disposition, and Content-Length, and keeps every
// other header the caller set. It returns nil once the header is out, even
// if the write fails, so that the Errors middleware does not append JSON to
// the file. The client detects a short body from Content-Length.
func RespondFile(ctx context.Context, w http.ResponseWriter, contentType, filename string, data []byte) error {
	setAttachmentHeader(w.Header(), contentType, filename)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))

	mux.SetStatusCode(ctx, http.StatusOK)
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write(data)

	return nil
}

// RespondStream copies body as a 200 attachment named filename, with the
// same headers as RespondFile except Content-Length. A nil log uses
// slog.Default().
//
// It reads the first bytes before it writes the header, so a read error in
// that read returns the error and writes nothing. An empty body writes the
// header and returns nil. After the header is out, a read or write error is
// logged to log and RespondStream panics with http.ErrAbortHandler, so the
// server aborts the connection and the client does not take a cut-short body
// for a complete file.
func RespondStream(ctx context.Context, log *slog.Logger, w http.ResponseWriter, contentType, filename string, body io.Reader) error {
	if log == nil {
		log = slog.Default()
	}

	buf := make([]byte, 32<<10)

	var n int
	var readErr error
	for n == 0 && readErr == nil {
		n, readErr = body.Read(buf)
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}

	setAttachmentHeader(w.Header(), contentType, filename)

	mux.SetStatusCode(ctx, http.StatusOK)
	w.WriteHeader(http.StatusOK)

	_, err := w.Write(buf[:n])
	if err == nil && readErr == nil {
		_, err = io.CopyBuffer(w, body, buf)
	}
	if err != nil {
		log.ErrorContext(ctx, "respond stream", "trace_id", mux.GetTraceID(ctx), "error", err)
		panic(http.ErrAbortHandler)
	}

	return nil
}

// setAttachmentHeader sets Content-Type and an attachment Content-Disposition.
func setAttachmentHeader(h http.Header, contentType, filename string) {
	disposition := "attachment"
	if filename != "" {
		if v := mime.FormatMediaType("attachment", map[string]string{"filename": filename}); v != "" {
			disposition = v
		}
	}

	h.Set("Content-Type", contentType)
	h.Set("Content-Disposition", disposition)
}
