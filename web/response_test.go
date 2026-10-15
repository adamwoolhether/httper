package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/errs"
	"github.com/adamwoolhether/httper/web/middleware"
	"github.com/adamwoolhether/httper/web/mux"
)

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.Background()

	data := map[string]string{"status": "ok"}
	err := web.RespondJSON(ctx, w, http.StatusOK, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", ct, "application/json")
	}

	var m map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if m["status"] != "ok" {
		t.Fatalf("body status = %q, want %q", m["status"], "ok")
	}
}

func TestRespondJSON_NoContent(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.Background()

	err := web.RespondJSON(ctx, w, http.StatusNoContent, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("body should be empty, got %d bytes", w.Body.Len())
	}
}

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.Background()

	appErr := errs.New(http.StatusBadRequest, fmt.Errorf("bad input"))
	err := web.RespondError(ctx, w, appErr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m["message"] != "bad input" {
		t.Fatalf("message = %v, want %q", m["message"], "bad input")
	}
	if int(m["code"].(float64)) != http.StatusBadRequest {
		t.Fatalf("code = %v, want %d", m["code"], http.StatusBadRequest)
	}
}

func TestRedirect(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/old", nil)

	err := web.Redirect(w, r, "/new", http.StatusFound)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/new" {
		t.Fatalf("Location = %q, want %q", loc, "/new")
	}
}

func TestRedirect_InvalidCode(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/old", nil)

	tests := map[string]int{
		"200": http.StatusOK,
		"404": http.StatusNotFound,
		"500": http.StatusInternalServerError,
	}

	for name, code := range tests {
		t.Run(name, func(t *testing.T) {
			if err := web.Redirect(w, r, "/new", code); err == nil {
				t.Fatalf("expected error for non-3xx code %d", code)
			}
		})
	}
}

func TestRespondFile(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Cache-Control", "no-store")
	data := []byte("id,name\n1,alice\n")

	if err := web.RespondFile(context.Background(), w, "text/csv", "report.csv", data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	wantHeader := map[string]string{
		"Content-Type":        "text/csv",
		"Content-Disposition": "attachment; filename=report.csv",
		"Content-Length":      strconv.Itoa(len(data)),
		"Cache-Control":       "no-store",
	}
	for key, want := range wantHeader {
		if got := w.Header().Get(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}

	if !bytes.Equal(w.Body.Bytes(), data) {
		t.Fatalf("body = %q, want %q", w.Body.Bytes(), data)
	}
}

func TestRespondFile_Filename(t *testing.T) {
	tests := map[string]string{
		"ascii":     "report.csv",
		"space":     "my report.csv",
		"non-ascii": "résumé 履歴書.pdf",
		"quote":     `say "hi".txt`,
	}

	for name, filename := range tests {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()

			if err := web.RespondFile(context.Background(), w, "application/octet-stream", filename, []byte("x")); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			disposition, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
			if err != nil {
				t.Fatalf("parse Content-Disposition %q: %v", w.Header().Get("Content-Disposition"), err)
			}
			if disposition != "attachment" {
				t.Fatalf("disposition = %q, want %q", disposition, "attachment")
			}
			if params["filename"] != filename {
				t.Fatalf("filename = %q, want %q", params["filename"], filename)
			}
		})
	}
}

func TestRespondFile_EmptyFilename(t *testing.T) {
	w := httptest.NewRecorder()

	if err := web.RespondFile(context.Background(), w, "application/pdf", "", []byte("%PDF")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := w.Header().Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("Content-Disposition = %q, want %q", got, "attachment")
	}
}

func TestRespondFile_WriteError(t *testing.T) {
	w := &failingWriter{ResponseRecorder: httptest.NewRecorder()}

	if err := web.RespondFile(context.Background(), w, "text/plain", "a.txt", []byte("data")); err != nil {
		t.Fatalf("error = %v, want nil after the header is out", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRespond_StatusCode(t *testing.T) {
	tests := map[string]mux.Handler{
		"file": func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			return web.RespondFile(ctx, w, "text/plain", "a.txt", []byte("data"))
		},
		"stream": func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			return web.RespondStream(ctx, slog.New(slog.DiscardHandler), w, "text/plain", "a.txt", strings.NewReader("data"))
		},
	}

	for name, handler := range tests {
		t.Run(name, func(t *testing.T) {
			log, buf := newTestLogger(t)
			app := mux.New(mux.WithMiddleware(middleware.Logger(log)))
			app.Get("/download", handler)

			w := httptest.NewRecorder()
			app.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/download", nil))

			if !strings.Contains(buf.String(), "statusCode=200") {
				t.Fatalf("request log should record statusCode=200:\n%s", buf.String())
			}
		})
	}
}

func TestRespondStream(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Cache-Control", "no-store")
	log, buf := newTestLogger(t)

	err := web.RespondStream(context.Background(), log, w, "text/csv", "report.csv", strings.NewReader("id,name\n1,alice\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	wantHeader := map[string]string{
		"Content-Type":        "text/csv",
		"Content-Disposition": "attachment; filename=report.csv",
		"Content-Length":      "",
		"Cache-Control":       "no-store",
	}
	for key, want := range wantHeader {
		if got := w.Header().Get(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}

	if got := w.Body.String(); got != "id,name\n1,alice\n" {
		t.Fatalf("body = %q, want %q", got, "id,name\n1,alice\n")
	}
	if buf.Len() != 0 {
		t.Fatalf("unexpected log output: %s", buf.String())
	}
}

func TestRespondStream_ReadErrorBeforeFirstByte(t *testing.T) {
	errRead := errors.New("storage unavailable")
	tests := map[string]io.Reader{
		"first read":     iotest.ErrReader(errRead),
		"after no bytes": &emptyReads{n: 3, err: errRead},
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx := context.Background()
			log, buf := newTestLogger(t)

			err := web.RespondStream(ctx, log, w, "text/csv", "report.csv", body)
			if !errors.Is(err, errRead) {
				t.Fatalf("error = %v, want %v", err, errRead)
			}
			if got := w.Header().Get("Content-Disposition"); got != "" {
				t.Fatalf("Content-Disposition = %q, want none", got)
			}
			if buf.Len() != 0 {
				t.Fatalf("unexpected log output: %s", buf.String())
			}

			// The header is not out, so an error response still sets the status.
			if err := web.RespondJSON(ctx, w, http.StatusInternalServerError, map[string]string{"error": "x"}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
			}
		})
	}
}

func TestRespondStream_EmptyBody(t *testing.T) {
	w := httptest.NewRecorder()
	log, _ := newTestLogger(t)

	if err := web.RespondStream(context.Background(), log, w, "text/csv", "empty.csv", strings.NewReader("")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Header().Get("Content-Disposition"); got != "attachment; filename=empty.csv" {
		t.Fatalf("Content-Disposition = %q, want %q", got, "attachment; filename=empty.csv")
	}
	if w.Body.Len() != 0 {
		t.Fatalf("body should be empty, got %d bytes", w.Body.Len())
	}
}

func TestRespondStream_ErrorAfterHeader(t *testing.T) {
	errRead := errors.New("storage unavailable")
	tests := map[string]struct {
		w    func() http.ResponseWriter
		body func() io.Reader
	}{
		"read error after first chunk": {
			w:    func() http.ResponseWriter { return httptest.NewRecorder() },
			body: func() io.Reader { return io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(errRead)) },
		},
		"read error with first chunk": {
			w:    func() http.ResponseWriter { return httptest.NewRecorder() },
			body: func() io.Reader { return &dataErrReader{data: []byte("partial"), err: errRead} },
		},
		"write error": {
			w:    func() http.ResponseWriter { return &failingWriter{ResponseRecorder: httptest.NewRecorder()} },
			body: func() io.Reader { return strings.NewReader("data") },
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			log, buf := newTestLogger(t)

			var rec any
			func() {
				defer func() { rec = recover() }()
				_ = web.RespondStream(context.Background(), log, tc.w(), "text/csv", "report.csv", tc.body())
			}()

			if rec != http.ErrAbortHandler {
				t.Fatalf("panic value = %v, want http.ErrAbortHandler", rec)
			}
			if got := strings.Count(buf.String(), "level=ERROR"); got != 1 {
				t.Fatalf("log has %d error entries, want 1:\n%s", got, buf.String())
			}
			if !strings.Contains(buf.String(), "trace_id=") {
				t.Fatalf("expected trace_id in log output: %s", buf.String())
			}
		})
	}
}

func TestRespondStream_Server(t *testing.T) {
	data := make([]byte, 100<<10+7)
	for i := range data {
		data[i] = byte(i)
	}

	tests := map[string]struct {
		body    func() io.Reader
		wantErr bool
	}{
		"larger than buffer": {
			body: func() io.Reader { return iotest.HalfReader(bytes.NewReader(data)) },
		},
		"read error after header": {
			body: func() io.Reader {
				return io.MultiReader(bytes.NewReader(data), iotest.ErrReader(errors.New("storage unavailable")))
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			log, _ := newTestLogger(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = web.RespondStream(r.Context(), log, w, "application/octet-stream", "data.bin", tc.body())
			}))
			defer srv.Close()

			got, err := get(srv.URL)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("client read %d bytes without error, want a failed read", len(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("body has %d bytes, want the %d sent bytes", len(got), len(data))
			}
		})
	}
}

func get(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// failingWriter records the header but fails every body write.
type failingWriter struct {
	*httptest.ResponseRecorder
}

func (w *failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("connection reset")
}

// emptyReads returns no bytes and no error n times, then err.
type emptyReads struct {
	n   int
	err error
}

func (r *emptyReads) Read([]byte) (int, error) {
	if r.n == 0 {
		return 0, r.err
	}
	r.n--

	return 0, nil
}

// dataErrReader returns data and err from the same Read call.
type dataErrReader struct {
	data []byte
	err  error
}

func (r *dataErrReader) Read(p []byte) (int, error) {
	return copy(p, r.data), r.err
}

func newTestLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	t.Cleanup(func() {
		if os.Getenv("VERBOSE") != "" {
			t.Log(buf.String())
		}
	})
	return log, &buf
}
