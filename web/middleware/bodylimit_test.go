package middleware_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/middleware"
	"github.com/adamwoolhether/httper/web/mux"
)

func TestBodyLimit(t *testing.T) {
	const limit = 8

	tests := map[string]struct {
		body       string
		wantStatus int
		wantRead   string
		wantBody   string
	}{
		"under limit": {body: "1234567", wantStatus: http.StatusNoContent, wantRead: "1234567"},
		"at limit":    {body: "12345678", wantStatus: http.StatusNoContent, wantRead: "12345678"},
		"over limit": {
			body:       "123456789",
			wantStatus: http.StatusRequestEntityTooLarge,
			wantBody:   `{"code":413,"message":"request body exceeds 8 bytes"}`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			log, _ := newTestLogger(t)

			var read string
			app := mux.New(mux.WithMiddleware(middleware.Errors(log), middleware.BodyLimit(limit)))
			app.Post("/upload", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				data, err := io.ReadAll(r.Body)
				if err != nil {
					return fmt.Errorf("read body: %w", err)
				}
				read = string(data)

				return web.RespondJSON(ctx, w, http.StatusNoContent, nil)
			})

			w := httptest.NewRecorder()
			app.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(tc.body)))

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if read != tc.wantRead {
				t.Fatalf("handler read %q, want %q", read, tc.wantRead)
			}
			if got := w.Body.String(); got != tc.wantBody {
				t.Fatalf("body = %s, want %s", got, tc.wantBody)
			}
		})
	}
}
