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
	"time"

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

func TestRespond_EmptyContentType(t *testing.T) {
	tests := map[string]func(w http.ResponseWriter) error{
		"file": func(w http.ResponseWriter) error {
			return web.RespondFile(context.Background(), w, "", "a.bin", []byte("data"))
		},
		"stream": func(w http.ResponseWriter) error {
			return web.RespondStream(context.Background(), slog.New(slog.DiscardHandler), w, "", "a.bin", strings.NewReader("data"))
		},
	}

	for name, respond := range tests {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()

			if err := respond(w); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := w.Header().Get("Content-Type"); got != "application/octet-stream" {
				t.Fatalf("Content-Type = %q, want %q", got, "application/octet-stream")
			}
		})
	}
}

func TestRespondFile_WriteError(t *testing.T) {
	w := httptest.NewRecorder()

	if err := web.RespondFile(context.Background(), failingWriter{w}, "text/plain", "a.txt", []byte("data")); err != nil {
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

func TestRespondStream_ReadErrorBeforeHeader(t *testing.T) {
	errRead := errors.New("storage unavailable")
	tests := map[string]io.Reader{
		"first read":       iotest.ErrReader(errRead),
		"after no bytes":   &emptyReads{n: 3, err: errRead},
		"with first bytes": &dataErrReader{data: []byte("partial"), err: errRead},
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
	tests := map[string]struct {
		w       func(http.ResponseWriter) http.ResponseWriter
		body    io.Reader
		wantLog string
	}{
		"read error": {
			w:       func(w http.ResponseWriter) http.ResponseWriter { return w },
			body:    io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(errors.New("storage unavailable"))),
			wantLog: "storage unavailable",
		},
		"write error": {
			w:       func(w http.ResponseWriter) http.ResponseWriter { return failingWriter{w} },
			body:    strings.NewReader("data"),
			wantLog: "connection reset",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			log, buf := newTestLogger(t)

			var traceID string
			app := mux.New()
			app.Get("/download", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				traceID = mux.GetTraceID(ctx)
				return web.RespondStream(ctx, log, tc.w(w), "text/csv", "report.csv", tc.body)
			})

			var rec any
			func() {
				defer func() { rec = recover() }()
				app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/download", nil))
			}()

			if rec != http.ErrAbortHandler {
				t.Fatalf("panic value = %v, want http.ErrAbortHandler", rec)
			}
			if got := strings.Count(buf.String(), "level=ERROR"); got != 1 {
				t.Fatalf("log has %d error entries, want 1:\n%s", got, buf.String())
			}
			for _, want := range []string{"trace_id=" + traceID, tc.wantLog} {
				if !strings.Contains(buf.String(), want) {
					t.Fatalf("log should contain %q:\n%s", want, buf.String())
				}
			}
		})
	}
}

func TestRespondStream_LogContext(t *testing.T) {
	handler := &ctxHandler{}

	var traceID string
	app := mux.New()
	app.Get("/download", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		traceID = mux.GetTraceID(ctx)
		body := io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(errors.New("storage unavailable")))
		return web.RespondStream(ctx, slog.New(handler), w, "text/csv", "report.csv", body)
	})

	func() {
		defer func() { _ = recover() }()
		app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/download", nil))
	}()

	if handler.ctx == nil {
		t.Fatal("log handler received no record")
	}
	if got := mux.GetTraceID(handler.ctx); got != traceID {
		t.Fatalf("log handler context trace ID = %q, want the request trace ID %q", got, traceID)
	}
}

func TestRespondStream_NilLog(t *testing.T) {
	body := io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(errors.New("storage unavailable")))

	var rec any
	func() {
		defer func() { rec = recover() }()
		_ = web.RespondStream(context.Background(), nil, httptest.NewRecorder(), "text/csv", "report.csv", body)
	}()

	if rec != http.ErrAbortHandler {
		t.Fatalf("panic value = %v, want http.ErrAbortHandler", rec)
	}
}

func TestRespondStream_LargeBody(t *testing.T) {
	data := testData(100<<10 + 7)
	log, _ := newTestLogger(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = web.RespondStream(r.Context(), log, w, "application/octet-stream", "data.bin", iotest.HalfReader(bytes.NewReader(data)))
	}))
	defer srv.Close()

	got, err := get(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("body has %d bytes, want the %d sent bytes", len(got), len(data))
	}
}

func TestRespondStream_AbortReachesClient(t *testing.T) {
	data := testData(100<<10 + 7)
	log, _ := newTestLogger(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := io.MultiReader(bytes.NewReader(data), iotest.ErrReader(errors.New("storage unavailable")))
		_ = web.RespondStream(r.Context(), log, w, "application/octet-stream", "data.bin", body)
	}))
	defer srv.Close()

	if got, err := get(srv.URL); err == nil {
		t.Fatalf("client read %d bytes without error, want a failed read", len(got))
	}
}

func TestRespondStream_CallerContentLength(t *testing.T) {
	data := testData(100<<10 + 7)
	short := len(data) - 1

	// Each part ends a Read, so a separate part puts the extra bytes in a new Read.
	tests := map[string]struct {
		contentLength int
		parts         [][]byte
		wantErr       bool
		want          []byte
	}{
		"matches body":                    {contentLength: len(data), parts: [][]byte{data}, want: data},
		"body longer, within a read":      {contentLength: short, parts: [][]byte{data}, wantErr: true},
		"body longer, at a read boundary": {contentLength: short, parts: [][]byte{data[:short], data[short:]}, want: data[:short]},
		"body shorter":                    {contentLength: len(data) + 1, parts: [][]byte{data}, wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			log, _ := newTestLogger(t)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", strconv.Itoa(tc.contentLength))
				parts := make([]io.Reader, len(tc.parts))
				for i, part := range tc.parts {
					parts[i] = bytes.NewReader(part)
				}
				// A plain reader, as from a file or object store, without a WriteTo method.
				body := struct{ io.Reader }{io.MultiReader(parts...)}
				_ = web.RespondStream(r.Context(), log, w, "application/octet-stream", "data.bin", body)
			}))
			defer srv.Close()

			got, err := get(srv.URL)
			if (err != nil) != tc.wantErr {
				t.Fatalf("client error = %v, want error %t", err, tc.wantErr)
			}
			if err == nil && !bytes.Equal(got, tc.want) {
				t.Fatalf("body has %d bytes, want %d bytes", len(got), len(tc.want))
			}
		})
	}
}

func TestRespondStream_NoProgress(t *testing.T) {
	tests := map[string]struct {
		emptyReads int
		wantErr    error
	}{
		"99 empty reads, then EOF": {emptyReads: 99},
		"100 empty reads":          {emptyReads: 100, wantErr: io.ErrNoProgress},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			log, _ := newTestLogger(t)
			body := &emptyReads{n: tc.emptyReads, err: io.EOF}

			err := web.RespondStream(context.Background(), log, w, "text/csv", "report.csv", body)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}

			wantDisposition := "attachment; filename=report.csv"
			if tc.wantErr != nil {
				wantDisposition = ""
			}
			if got := w.Header().Get("Content-Disposition"); got != wantDisposition {
				t.Fatalf("Content-Disposition = %q, want %q", got, wantDisposition)
			}
		})
	}
}

func TestRespondStream_ClientGone(t *testing.T) {
	// net/http watches the connection for a closed client only after it reads
	// the request body to the end, and it does not drain an unread body over
	// 256 KiB. With a Content-Length, its ReadFrom writes to the connection
	// directly, and a failed write there leaves ctx live. So in the POST case,
	// only the writes of RespondStream can cancel ctx.
	tests := map[string]func(url string) (*http.Response, error){
		"GET": http.Get,
		"POST with a large unread body": func(url string) (*http.Response, error) {
			return http.Post(url, "text/plain", strings.NewReader(strings.Repeat("x", 1<<20)))
		},
	}

	for name, send := range tests {
		t.Run(name, func(t *testing.T) {
			log, buf := newTestLogger(t)

			done := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(done)
				w.Header().Set("Content-Length", strconv.FormatInt(1<<40, 10))
				_ = web.RespondStream(r.Context(), log, w, "application/octet-stream", "data.bin", endlessReader{})
			}))
			defer srv.Close()

			resp, err := send(srv.URL)
			if err != nil {
				t.Fatalf("send: %v", err)
			}
			if _, err := resp.Body.Read(make([]byte, 1)); err != nil {
				t.Fatalf("read: %v", err)
			}
			resp.Body.Close() // closes the connection, since the body is unread

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("handler did not end after the client went away")
			}

			if !strings.Contains(buf.String(), "level=WARN") || strings.Contains(buf.String(), "level=ERROR") {
				t.Fatalf("log should have a WARN entry and no ERROR entry:\n%s", buf.String())
			}
		})
	}
}

func testData(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i)
	}

	return data
}

func get(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// failingWriter passes the header through but fails every body write.
type failingWriter struct {
	http.ResponseWriter
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("connection reset")
}

// endlessReader fills every Read with bytes and never ends.
type endlessReader struct{}

func (endlessReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}

	return len(p), nil
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

// ctxHandler is a slog.Handler that keeps the context of the last record.
type ctxHandler struct {
	ctx context.Context
}

func (h *ctxHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *ctxHandler) Handle(ctx context.Context, _ slog.Record) error {
	h.ctx = ctx
	return nil
}

func (h *ctxHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *ctxHandler) WithGroup(string) slog.Handler { return h }

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
