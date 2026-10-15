package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/adamwoolhether/httper/web/middleware"
)

func TestDeadline(t *testing.T) {
	tests := map[string]struct {
		d       time.Duration
		wait    time.Duration
		wantErr error
	}{
		"write before deadline": {d: time.Minute},
		"write after deadline":  {d: 50 * time.Millisecond, wait: 250 * time.Millisecond, wantErr: os.ErrDeadlineExceeded},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			writeErr := make(chan error, 1)
			handler := middleware.Deadline(tc.d)(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				time.Sleep(tc.wait)
				w.Write([]byte("late"))
				err := http.NewResponseController(w).Flush()
				writeErr <- err
				return err
			})

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := handler(r.Context(), w, r); err != nil {
					t.Logf("handler: %v", err)
				}
			}))
			defer srv.Close()

			if resp, err := srv.Client().Get(srv.URL); err == nil {
				resp.Body.Close()
			}

			if err := <-writeErr; !errors.Is(err, tc.wantErr) {
				t.Fatalf("write error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestDeadline_NotSupported(t *testing.T) {
	called := false
	handler := middleware.Deadline(time.Second)(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		called = true
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	err := handler(r.Context(), w, r)
	if !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("error = %v, want %v", err, http.ErrNotSupported)
	}
	if called {
		t.Fatal("handler ran without a deadline")
	}
}
