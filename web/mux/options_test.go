package mux_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/adamwoolhether/httper/web/middleware"
	"github.com/adamwoolhether/httper/web/mux"
)

func TestWithMiddleware_AutoGlobalCORS(t *testing.T) {
	app := mux.New(mux.WithMiddleware(middleware.CORS([]string{"*"})))
	app.Get("/ping", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/ping", nil)
	req.Header.Set("Origin", "http://example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /ping: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "http://example.com")
	}
}

func TestWithMiddleware_AutoGlobalCSRF(t *testing.T) {
	log := slog.Default()
	app := mux.New(mux.WithMiddleware(middleware.CSRF(log)))
	app.Get("/safe", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
		return nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	// GET is a safe method; CSRF should allow it through.
	resp, err := http.Get(srv.URL + "/safe")
	if err != nil {
		t.Fatalf("GET /safe: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestWithMiddleware_SortsRoute(t *testing.T) {
	log := slog.Default()

	// Pass Logger, Errors, Panics in reverse priority order.
	// WithMiddleware should sort them: Logger(3), Errors(4), Panics(100).
	app := mux.New(mux.WithMiddleware(
		middleware.Panics(),
		middleware.Errors(log),
		middleware.Logger(log),
	))

	app.Get("/test", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/test")
	if err != nil {
		t.Fatalf("GET /test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestWithMiddleware_CustomRoute(t *testing.T) {
	customMW := func(handler mux.Handler) mux.Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("X-Custom", "applied")
			return handler(ctx, w, r)
		}
	}

	app := mux.New(mux.WithMiddleware(customMW))
	app.Get("/ping", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ping")
	if err != nil {
		t.Fatalf("GET /ping: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Custom"); got != "applied" {
		t.Fatalf("X-Custom = %q, want %q", got, "applied")
	}
}

func TestWithMiddleware_MixedGlobalAndRoute(t *testing.T) {
	customMW := func(handler mux.Handler) mux.Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("X-Route", "yes")
			return handler(ctx, w, r)
		}
	}

	app := mux.New(mux.WithMiddleware(
		middleware.CORS([]string{"*"}),
		middleware.Panics(),
		customMW,
	))
	app.Get("/mixed", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/mixed", nil)
	req.Header.Set("Origin", "http://example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /mixed: %v", err)
	}
	defer resp.Body.Close()

	// CORS should run as global middleware.
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "http://example.com")
	}

	// Custom route middleware should also run.
	if got := resp.Header.Get("X-Route"); got != "yes" {
		t.Fatalf("X-Route = %q, want %q", got, "yes")
	}
}

func TestWithTracer(t *testing.T) {
	// Passing nil tracer leaves the noop tracer in place.
	// The key check is that New doesn't panic.
	app := mux.New()
	app.Get("/ok", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		v := mux.GetValues(ctx)
		if v.Tracer == nil {
			t.Error("Tracer should not be nil")
		}
		w.WriteHeader(http.StatusOK)
		return nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ok")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
}

func TestWithStaticFS(t *testing.T) {
	fs := fstest.MapFS{
		"hello.txt": &fstest.MapFile{Data: []byte("hello world")},
	}

	app := mux.New(mux.WithStaticFS(fs, "/static/"))
	srv := httptest.NewServer(app)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/static/hello.txt")
	if err != nil {
		t.Fatalf("GET /static/hello.txt: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestAdapt(t *testing.T) {
	stdHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Adapted", "yes")
		w.WriteHeader(http.StatusAccepted)
	})

	adapted := mux.Adapt(stdHandler)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	err := adapted(r.Context(), w, r)
	if err != nil {
		t.Fatalf("Adapt handler returned error: %v", err)
	}
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
	if got := w.Header().Get("X-Adapted"); got != "yes" {
		t.Fatalf("X-Adapted = %q, want %q", got, "yes")
	}
}
