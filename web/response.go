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
// other header the caller set. An empty contentType sends
// application/octet-stream. It returns nil once the header is out, even
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

// maxEmptyReads matches the limit that bufio.Reader puts on reads with no progress.
const maxEmptyReads = 100

// RespondStream copies body as a 200 attachment named filename, with the
// same headers as RespondFile except Content-Length. A nil log uses
// slog.Default().
//
// A Content-Length that the caller set stays. When body is shorter, the client
// gets a failed read. When body is longer, the client can take the first
// Content-Length bytes for a complete file, so set Content-Length only to the
// exact length of body.
//
// It reads the first bytes before it writes the header, so a read error in
// that read returns the error and writes nothing. RespondStream treats 100
// reads in a row with no bytes and no error as the read error
// io.ErrNoProgress. An empty body writes the header and returns nil. After the
// header is out, a read or write error is logged to log and RespondStream
// panics with http.ErrAbortHandler, so the server aborts the connection and
// the client does not take a cut-short body for a complete file. The log level
// is Warn when ctx is done, and Error otherwise. On HTTP/1, a failed write to
// the connection cancels ctx, so a client that goes away and a write past a
// deadline both log at Warn. Over HTTP/2, a dropped connection can still log
// at Error.
//
// A GET route also serves HEAD. For a HEAD request, net/http drops the body,
// but RespondStream still reads all of body. Check r.Method before opening an
// expensive source.
func RespondStream(ctx context.Context, log *slog.Logger, w http.ResponseWriter, contentType, filename string, body io.Reader) error {
	if log == nil {
		log = slog.Default()
	}

	buf := make([]byte, 32<<10)
	body = &progressReader{r: body}

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
		// Hide w's ReadFrom: it writes to the connection directly, and a failed
		// write there does not cancel ctx.
		_, err = io.CopyBuffer(struct{ io.Writer }{w}, body, buf)
	}
	if err != nil {
		level := slog.LevelError
		if ctx.Err() != nil {
			level = slog.LevelWarn
		}
		log.Log(ctx, level, "respond stream", "trace_id", mux.GetTraceID(ctx), "error", err)
		panic(http.ErrAbortHandler)
	}

	return nil
}

// progressReader returns io.ErrNoProgress once maxEmptyReads reads in a row
// return no bytes and no error.
type progressReader struct {
	r     io.Reader
	empty int
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 || err != nil {
		p.empty = 0
		return n, err
	}

	p.empty++
	if p.empty == maxEmptyReads {
		return 0, io.ErrNoProgress
	}

	return 0, nil
}

// setAttachmentHeader sets Content-Type and an attachment Content-Disposition.
func setAttachmentHeader(h http.Header, contentType, filename string) {
	disposition := "attachment"
	if filename != "" {
		if v := mime.FormatMediaType("attachment", map[string]string{"filename": filename}); v != "" {
			disposition = v
		}
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	h.Set("Content-Type", contentType)
	h.Set("Content-Disposition", disposition)
}
