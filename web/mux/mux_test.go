package mux_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adamwoolhether/httper/web/middleware"
	"github.com/adamwoolhether/httper/web/mux"
)

func TestNew(t *testing.T) {
	app := mux.New()
	app.Get("/health", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
		return nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("body = %q, want %q", body, "ok")
	}
}

func TestApp_HTTPMethods(t *testing.T) {
	tests := map[string]struct {
		register func(*mux.App, string, mux.Handler, ...mux.Middleware)
		method   string
	}{
		"GET":    {register: (*mux.App).Get, method: http.MethodGet},
		"POST":   {register: (*mux.App).Post, method: http.MethodPost},
		"PUT":    {register: (*mux.App).Put, method: http.MethodPut},
		"PATCH":  {register: (*mux.App).Patch, method: http.MethodPatch},
		"DELETE": {register: (*mux.App).Delete, method: http.MethodDelete},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			app := mux.New()
			tc.register(app, "/test", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tc.method))
				return nil
			})

			srv := httptest.NewServer(app)
			defer srv.Close()

			req, _ := http.NewRequest(tc.method, srv.URL+"/test", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s /test: %v", tc.method, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}

			body, _ := io.ReadAll(resp.Body)
			if string(body) != tc.method {
				t.Fatalf("body = %q, want %q", body, tc.method)
			}
		})
	}
}

func TestApp_WrongMethod(t *testing.T) {
	app := mux.New()
	app.Get("/only-get", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/only-get", "", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestApp_Group_SharesMux(t *testing.T) {
	app := mux.New()
	g := app.Group()

	g.Get("/from-group", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("group"))
		return nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/from-group")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "group" {
		t.Fatalf("body = %q, want %q", body, "group")
	}
}

func TestApp_Group_IndependentMiddleware(t *testing.T) {
	app := mux.New()

	g := app.Group()
	groupMW := func(handler mux.Handler) mux.Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("X-Group-MW", "yes")
			return handler(ctx, w, r)
		}
	}
	g.Use(groupMW)

	g.Get("/with-mw", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})
	app.Get("/without-mw", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	// Group route should have the header.
	resp, _ := http.Get(srv.URL + "/with-mw")
	resp.Body.Close()
	if resp.Header.Get("X-Group-MW") != "yes" {
		t.Fatal("group route missing X-Group-MW header")
	}

	// Parent route should NOT have the header.
	resp, _ = http.Get(srv.URL + "/without-mw")
	resp.Body.Close()
	if resp.Header.Get("X-Group-MW") != "" {
		t.Fatal("parent route should not have X-Group-MW header")
	}
}

func TestApp_Mount_Prefix(t *testing.T) {
	app := mux.New()
	api := app.Mount("/api")

	api.Get("/users", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("users"))
		return nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/users")
	if err != nil {
		t.Fatalf("GET /api/users: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "users" {
		t.Fatalf("body = %q, want %q", body, "users")
	}
}

func TestApp_Mount_LeadingSlash(t *testing.T) {
	// Both Mount("/api") and Mount("api") should produce /api/…
	for _, prefix := range []string{"/api", "api"} {
		t.Run(prefix, func(t *testing.T) {
			app := mux.New()
			sub := app.Mount(prefix)
			sub.Get("/items", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				w.WriteHeader(http.StatusOK)
				return nil
			})

			srv := httptest.NewServer(app)
			defer srv.Close()

			resp, err := http.Get(srv.URL + "/api/items")
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
		})
	}
}

func TestApp_Use(t *testing.T) {
	app := mux.New()
	app.Use(func(handler mux.Handler) mux.Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("X-Used", "true")
			return handler(ctx, w, r)
		}
	})

	app.Get("/used", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/used")
	resp.Body.Close()

	if resp.Header.Get("X-Used") != "true" {
		t.Fatal("middleware added via Use should run")
	}
}

func TestApp_MiddlewareOrder(t *testing.T) {
	var order []string

	// CORS is auto-detected as global middleware by WithMiddleware.
	globalCORS := middleware.CORS([]string{"*"})

	appMW := func(handler mux.Handler) mux.Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			order = append(order, "app")
			return handler(ctx, w, r)
		}
	}

	routeMW := func(handler mux.Handler) mux.Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			order = append(order, "route")
			return handler(ctx, w, r)
		}
	}

	app := mux.New(mux.WithMiddleware(globalCORS))
	app.Use(appMW)
	app.Get("/ordered", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		order = append(order, "handler")
		w.WriteHeader(http.StatusOK)
		return nil
	}, routeMW)

	srv := httptest.NewServer(app)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/ordered", nil)
	req.Header.Set("Origin", "http://example.com")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	// Global CORS runs in ServeHTTP (no order tracking), then app, route, handler.
	expected := []string{"app", "route", "handler"}
	if len(order) != len(expected) {
		t.Fatalf("order = %v, want %v", order, expected)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Fatalf("order[%d] = %q, want %q", i, order[i], v)
		}
	}

	// Verify CORS ran as global middleware by checking the header.
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "http://example.com")
	}
}

func TestApp_RouteMiddleware(t *testing.T) {
	routeMW := func(handler mux.Handler) mux.Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("X-Route-MW", "yes")
			return handler(ctx, w, r)
		}
	}

	app := mux.New()
	app.Get("/with", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	}, routeMW)
	app.Get("/without", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/with")
	resp.Body.Close()
	if resp.Header.Get("X-Route-MW") != "yes" {
		t.Fatal("/with should have route MW header")
	}

	resp, _ = http.Get(srv.URL + "/without")
	resp.Body.Close()
	if resp.Header.Get("X-Route-MW") != "" {
		t.Fatal("/without should not have route MW header")
	}
}

func TestApp_ContextValues(t *testing.T) {
	app := mux.New()
	app.Get("/ctx", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		v := mux.GetValues(ctx)

		if v.TraceID == "" {
			t.Error("TraceID should be set")
		}
		if v.Now.IsZero() {
			t.Error("Now should be set")
		}
		if v.Tracer == nil {
			t.Error("Tracer should be set")
		}

		w.WriteHeader(http.StatusOK)
		return nil
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ctx")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
}

func TestApp_HandlerError(t *testing.T) {
	app := mux.New()
	app.Get("/err", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return fmt.Errorf("something went wrong")
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	// The server should not crash; it logs the error internally.
	resp, err := http.Get(srv.URL + "/err")
	if err != nil {
		t.Fatalf("GET /err: %v", err)
	}
	resp.Body.Close()
}
