package mux_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/errs"
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

func TestApp_Mount_Nested(t *testing.T) {
	app := mux.New()
	v1 := app.Mount("api").Mount("/v1/")
	v1.Get("users", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	tests := map[string]struct {
		path string
		want int
	}{
		"full prefix":  {path: "/api/v1/users", want: http.StatusOK},
		"inner prefix": {path: "/v1/users", want: http.StatusNotFound},
		"outer prefix": {path: "/api/users", want: http.StatusNotFound},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := serve(app, http.MethodGet, tc.path); got != tc.want {
				t.Fatalf("GET %s status = %d, want %d", tc.path, got, tc.want)
			}
		})
	}
}

func TestApp_Mount_Group(t *testing.T) {
	app := mux.New()
	api := app.Mount("/api")

	g := api.Group()
	g.Use(func(handler mux.Handler) mux.Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("X-Group-MW", "yes")
			return handler(ctx, w, r)
		}
	})
	g.Get("/health", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})
	api.Get("/users", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	w := httptest.NewRecorder()
	app.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/health status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get("X-Group-MW") != "yes" {
		t.Fatal("group route missing X-Group-MW header")
	}

	if got := serve(app, http.MethodGet, "/health"); got != http.StatusNotFound {
		t.Fatalf("GET /health status = %d, want %d", got, http.StatusNotFound)
	}

	w = httptest.NewRecorder()
	app.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/users status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get("X-Group-MW") != "" {
		t.Fatal("mounted route should not have X-Group-MW header")
	}
}

func TestApp_RoutePathNormalization(t *testing.T) {
	tests := map[string]struct {
		mount string
		route string
		path  string
		miss  string
	}{
		"root with slash":          {route: "/users", path: "/users"},
		"root without slash":       {route: "users", path: "/users"},
		"root empty route":         {route: "", path: "/", miss: "/other"},
		"root slash route":         {route: "/", path: "/other"},
		"mount with slashes":       {mount: "/api/", route: "/users", path: "/api/users"},
		"mount without slashes":    {mount: "api", route: "users", path: "/api/users"},
		"mount empty route":        {mount: "api", route: "", path: "/api", miss: "/api/other"},
		"mount slash route":        {mount: "api", route: "/", path: "/api/other"},
		"mount with path wildcard": {mount: "api", route: "users/{id}", path: "/api/users/42"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			app := mux.New()
			sub := app.Mount(tc.mount)
			sub.Get(tc.route, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				w.WriteHeader(http.StatusOK)
				return nil
			})

			if got := serve(app, http.MethodGet, tc.path); got != http.StatusOK {
				t.Fatalf("Mount(%q).Get(%q): GET %s status = %d, want %d", tc.mount, tc.route, tc.path, got, http.StatusOK)
			}
			if tc.miss == "" {
				return
			}
			if got := serve(app, http.MethodGet, tc.miss); got != http.StatusNotFound {
				t.Fatalf("Mount(%q).Get(%q): GET %s status = %d, want %d", tc.mount, tc.route, tc.miss, got, http.StatusNotFound)
			}
		})
	}
}

func TestApp_Handle_GroupNormalization(t *testing.T) {
	for _, group := range []string{"api/v1/", "/api//v1"} {
		t.Run(group, func(t *testing.T) {
			app := mux.New()
			app.Handle(http.MethodGet, group, "users", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				w.WriteHeader(http.StatusOK)
				return nil
			})

			if got := serve(app, http.MethodGet, "/api/v1/users"); got != http.StatusOK {
				t.Fatalf("GET /api/v1/users status = %d, want %d", got, http.StatusOK)
			}
		})
	}
}

func TestApp_VirtualHost(t *testing.T) {
	app := mux.New()
	app.Get("/users", body("any host"))
	app.VirtualHost("API.example.com").Get("users", body("api host"))
	app.VirtualHost("api.example.com").HandleNoMiddleware(http.MethodGet, "", "/raw", body("raw api host"))

	tests := map[string]struct {
		host string
		want string
	}{
		"matching host":       {host: "api.example.com", want: "api host"},
		"uppercase host":      {host: "API.EXAMPLE.COM", want: "api host"},
		"matching host, port": {host: "api.example.com:8443", want: "api host"},
		"other host":          {host: "other.example.com", want: "any host"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/users", nil)
			r.Host = tc.host
			w := httptest.NewRecorder()
			app.ServeHTTP(w, r)

			if w.Code != http.StatusOK || w.Body.String() != tc.want {
				t.Fatalf("Host %s: status = %d, body = %q, want 200 %q", tc.host, w.Code, w.Body.String(), tc.want)
			}
		})
	}

	for host, want := range map[string]int{"api.example.com": http.StatusOK, "other.example.com": http.StatusNotFound} {
		r := httptest.NewRequest(http.MethodGet, "/raw", nil)
		r.Host = host
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)

		if w.Code != want {
			t.Fatalf("HandleNoMiddleware: Host %s GET /raw status = %d, want %d", host, w.Code, want)
		}
	}
}

func TestApp_HandlerSeesLowercaseHost(t *testing.T) {
	app := mux.New()
	app.Get("/host", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		_, err := io.WriteString(w, r.Host)
		return err
	})

	r := httptest.NewRequest(http.MethodGet, "/host", nil)
	r.Host = "Mixed.Example.com:8443"
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)

	if got := w.Body.String(); got != "mixed.example.com:8443" {
		t.Fatalf("handler r.Host = %q, want %q", got, "mixed.example.com:8443")
	}
	if r.Host != "Mixed.Example.com:8443" {
		t.Fatalf("caller request Host = %q, want it unchanged", r.Host)
	}
	if r.Pattern != "GET /host" {
		t.Fatalf("caller request Pattern = %q, want %q", r.Pattern, "GET /host")
	}
}

func TestApp_VirtualHost_Composition(t *testing.T) {
	tests := map[string]func(app *mux.App) *mux.App{
		"host then mount":    func(app *mux.App) *mux.App { return app.VirtualHost("api.example.com").Mount("v1") },
		"mount then host":    func(app *mux.App) *mux.App { return app.Mount("v1").VirtualHost("api.example.com") },
		"host, mount, group": func(app *mux.App) *mux.App { return app.VirtualHost("api.example.com").Mount("v1").Group() },
	}

	for name, scope := range tests {
		t.Run(name, func(t *testing.T) {
			app := mux.New()
			scope(app).Get("users", body("ok"))

			for host, want := range map[string]int{"api.example.com": http.StatusOK, "other.example.com": http.StatusNotFound} {
				r := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
				r.Host = host
				w := httptest.NewRecorder()
				app.ServeHTTP(w, r)

				if w.Code != want {
					t.Fatalf("Host %s GET /v1/users status = %d, want %d", host, w.Code, want)
				}
			}
		})
	}
}

func TestApp_VirtualHost_Invalid(t *testing.T) {
	tests := map[string]string{
		"empty":            "",
		"scheme":           "https://api.example.com",
		"port":             "api.example.com:8443",
		"path":             "api.example.com/v1",
		"wildcard":         "*.example.com",
		"whitespace":       " ",
		"trailing space":   "api.example.com ",
		"ipv6 literal":     "[::1]",
		"non-ascii":        "bücher.example",
		"pattern wildcard": "{sub}.example.com",
		"kelvin sign":      "\u212Aiwi.example.com",
	}

	for name, host := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("VirtualHost(%q) did not panic", host)
				}
			}()
			mux.New().VirtualHost(host)
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
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "*")
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

// newFullStackApp creates an App wired with Logger → Errors → Panics and a
// captured log buffer for integration assertions.
func newFullStackApp(t *testing.T) (*mux.App, *httptest.Server, func() string) {
	t.Helper()
	log, buf := newTestLogger(t)
	app := mux.New(
		mux.WithLogger(log),
		mux.WithMiddleware(
			middleware.Logger(log),
			middleware.Errors(log),
			middleware.Panics(),
		),
	)
	srv := httptest.NewServer(app)
	t.Cleanup(srv.Close)
	return app, srv, buf.String
}

func TestApp_FullStack_Success(t *testing.T) {
	app, srv, logOutput := newFullStackApp(t)

	type payload struct {
		Msg string `json:"msg"`
	}
	app.Get("/ok", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return web.RespondJSON(ctx, w, http.StatusOK, payload{Msg: "hello"})
	})

	resp, err := http.Get(srv.URL + "/ok")
	if err != nil {
		t.Fatalf("GET /ok: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got payload
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.Msg != "hello" {
		t.Fatalf("body msg = %q, want %q", got.Msg, "hello")
	}

	logs := logOutput()
	if !strings.Contains(logs, "request started") {
		t.Fatal("log missing 'request started'")
	}
	if !strings.Contains(logs, "request completed") {
		t.Fatal("log missing 'request completed'")
	}
	if !strings.Contains(logs, "statusCode=200") {
		t.Fatalf("log missing statusCode=200, got:\n%s", logs)
	}
	if !strings.Contains(logs, "trace_id=") {
		t.Fatalf("log missing traceID, got:\n%s", logs)
	}
}

func TestApp_FullStack_AppError(t *testing.T) {
	app, srv, logOutput := newFullStackApp(t)

	app.Get("/bad", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return errs.New(http.StatusBadRequest, fmt.Errorf("bad input"))
	})

	resp, err := http.Get(srv.URL + "/bad")
	if err != nil {
		t.Fatalf("GET /bad: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if m["message"] != "bad input" {
		t.Fatalf("message = %v, want %q", m["message"], "bad input")
	}

	logs := logOutput()
	if !strings.Contains(logs, "statusCode=400") {
		t.Fatalf("log missing statusCode=400, got:\n%s", logs)
	}
	if !strings.Contains(logs, "trace_id=") {
		t.Fatalf("log missing traceID, got:\n%s", logs)
	}
}

func TestApp_FullStack_InternalError(t *testing.T) {
	app, srv, logOutput := newFullStackApp(t)

	app.Get("/internal", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return errs.NewInternal(fmt.Errorf("secret db error"))
	})

	resp, err := http.Get(srv.URL + "/internal")
	if err != nil {
		t.Fatalf("GET /internal: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}

	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if m["message"] != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("message = %v, want %q", m["message"], http.StatusText(http.StatusInternalServerError))
	}

	logs := logOutput()
	if !strings.Contains(logs, "statusCode=500") {
		t.Fatalf("log missing statusCode=500, got:\n%s", logs)
	}
	if !strings.Contains(logs, "secret db error") {
		t.Fatalf("log missing original error 'secret db error', got:\n%s", logs)
	}
	if !strings.Contains(logs, "trace_id=") {
		t.Fatalf("log missing traceID, got:\n%s", logs)
	}
}

func TestApp_FullStack_Panic(t *testing.T) {
	app, srv, logOutput := newFullStackApp(t)

	app.Get("/panic", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		panic("boom")
	})

	resp, err := http.Get(srv.URL + "/panic")
	if err != nil {
		t.Fatalf("GET /panic: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}

	logs := logOutput()
	if !strings.Contains(logs, "PANIC") {
		t.Fatalf("log missing PANIC, got:\n%s", logs)
	}
}

func TestApp_FullStack_FieldErrors(t *testing.T) {
	app, srv, logOutput := newFullStackApp(t)

	app.Get("/fields", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return errs.NewFieldsError("email", fmt.Errorf("required"))
	})

	resp, err := http.Get(srv.URL + "/fields")
	if err != nil {
		t.Fatalf("GET /fields: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnprocessableEntity)
	}

	var arr []map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&arr); err != nil {
		t.Fatalf("body should be JSON array: %v", err)
	}
	if len(arr) != 1 || arr[0]["field"] != "email" {
		t.Fatalf("unexpected body: %v", arr)
	}

	logs := logOutput()
	if !strings.Contains(logs, "statusCode=422") {
		t.Fatalf("log missing statusCode=422, got:\n%s", logs)
	}
}

func TestApp_FullStack_TraceIDInLogs(t *testing.T) {
	app, srv, logOutput := newFullStackApp(t)

	app.Get("/trace", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	resp, err := http.Get(srv.URL + "/trace")
	if err != nil {
		t.Fatalf("GET /trace: %v", err)
	}
	resp.Body.Close()

	logs := logOutput()

	if strings.Contains(logs, "trace_id=00000000000000000000000000000000") {
		t.Fatalf("trace_id should not be zero OTel trace ID, got:\n%s", logs)
	}

	// Both "request started" and "request completed" should have traceID.
	lines := strings.Split(logs, "\n")
	for _, line := range lines {
		if strings.Contains(line, "request started") || strings.Contains(line, "request completed") {
			if !strings.Contains(line, "trace_id=") {
				t.Fatalf("log line missing traceID: %s", line)
			}
		}
	}
}

func body(text string) mux.Handler {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		_, err := io.WriteString(w, text)
		return err
	}
}

func serve(app *mux.App, method, path string) int {
	w := httptest.NewRecorder()
	app.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w.Code
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
