package middleware_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"testing"
	"time"

	"github.com/adamwoolhether/httper/web/middleware"
	"github.com/adamwoolhether/httper/web/mux"
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

			srv := httptest.NewServer(serve(t, handler))
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

func TestDeadline_ReadBody(t *testing.T) {
	readErr := make(chan error, 1)
	handler := middleware.Deadline(50 * time.Millisecond)(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		_, err := io.ReadAll(r.Body)
		readErr <- err
		return err
	})

	srv := httptest.NewServer(serve(t, handler))
	defer srv.Close()

	// The client declares a body and never sends it.
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := io.WriteString(conn, "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 4\r\n\r\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}

	select {
	case err := <-readErr:
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("read error = %v, want %v", err, os.ErrDeadlineExceeded)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("body read did not end at the deadline")
	}
}

func TestDeadline_NoBodyKeepsConnection(t *testing.T) {
	slowDone := make(chan struct{})
	slow := middleware.Deadline(50 * time.Millisecond)(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		defer close(slowDone)

		w.Header().Set("Content-Length", "2")
		w.Write([]byte("ok"))
		if err := http.NewResponseController(w).Flush(); err != nil {
			return err
		}

		time.Sleep(150 * time.Millisecond)
		return nil
	})

	nextErr := make(chan error, 1)
	routes := http.NewServeMux()
	routes.Handle("GET /slow", serve(t, slow))
	routes.HandleFunc("GET /next", func(w http.ResponseWriter, r *http.Request) {
		nextErr <- r.Context().Err()
	})

	srv := httptest.NewServer(routes)
	defer srv.Close()

	if _, err := fetch(srv.Client(), srv.URL+"/slow", nil); err != nil {
		t.Fatalf("get /slow: %v", err)
	}
	<-slowDone

	var reused bool
	trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }}
	if _, err := fetch(srv.Client(), srv.URL+"/next", trace); err != nil {
		t.Fatalf("get /next: %v", err)
	}

	if !reused {
		t.Fatal("second request did not reuse the connection")
	}
	if err := <-nextErr; err != nil {
		t.Fatalf("next request context error = %v, want nil", err)
	}
}

func TestDeadline_NotSupported(t *testing.T) {
	tests := map[string]struct {
		w             func() http.ResponseWriter
		contentLength int64
		wantErr       error
		wantCalled    bool
	}{
		"no deadlines": {
			w:       func() http.ResponseWriter { return httptest.NewRecorder() },
			wantErr: http.ErrNotSupported,
		},
		"no read deadline, with body": {
			w:             func() http.ResponseWriter { return writeDeadlineOnly{httptest.NewRecorder()} },
			contentLength: 4,
			wantErr:       http.ErrNotSupported,
		},
		"no read deadline, without body": {
			w:          func() http.ResponseWriter { return writeDeadlineOnly{httptest.NewRecorder()} },
			wantCalled: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			called := false
			handler := middleware.Deadline(time.Second)(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				called = true
				return nil
			})

			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r.ContentLength = tc.contentLength

			if err := handler(r.Context(), tc.w(), r); !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if called != tc.wantCalled {
				t.Fatalf("handler called = %t, want %t", called, tc.wantCalled)
			}
		})
	}
}

// writeDeadlineOnly supports a write deadline but not a read deadline.
type writeDeadlineOnly struct {
	*httptest.ResponseRecorder
}

func (writeDeadlineOnly) SetWriteDeadline(time.Time) error {
	return nil
}

// serve adapts handler to an http.Handler and logs the error it returns.
func serve(t *testing.T, handler mux.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := handler(r.Context(), w, r); err != nil {
			t.Logf("handler: %v", err)
		}
	})
}

func fetch(client *http.Client, url string, trace *httptrace.ClientTrace) ([]byte, error) {
	ctx := context.Background()
	if trace != nil {
		ctx = httptrace.WithClientTrace(ctx, trace)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
