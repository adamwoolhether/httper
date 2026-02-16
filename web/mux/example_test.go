package mux_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing/fstest"

	"github.com/adamwoolhether/httper/web/middleware"
	"github.com/adamwoolhether/httper/web/mux"
)

func ExampleNew() {
	app := mux.New(
		mux.WithLogger(slog.Default()),
	)

	app.Get("/health", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		fmt.Fprint(w, "ok")
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	app.ServeHTTP(w, r)

	fmt.Println(w.Body.String())
	// Output: ok
}

// ————————————————————————————————————————————————————————————————————
// HTTP method handler examples
// ————————————————————————————————————————————————————————————————————

func ExampleApp_Get() {
	app := mux.New()
	app.Get("/hello", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		fmt.Fprint(w, "hello world")
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/hello", nil)
	app.ServeHTTP(w, r)

	fmt.Println(w.Code)
	fmt.Println(w.Body.String())
	// Output:
	// 200
	// hello world
}

func ExampleApp_Post() {
	app := mux.New()
	app.Post("/items", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "created")
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/items", nil)
	app.ServeHTTP(w, r)

	fmt.Println(w.Code)
	fmt.Println(w.Body.String())
	// Output:
	// 201
	// created
}

// ————————————————————————————————————————————————————————————————————
// Routing examples
// ————————————————————————————————————————————————————————————————————

func ExampleApp_Group() {
	app := mux.New()

	// Create a group with an independent middleware stack.
	api := app.Group()
	api.Use(func(handler mux.Handler) mux.Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("X-API", "true")
			return handler(ctx, w, r)
		}
	})

	api.Get("/api/data", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		fmt.Fprint(w, "data")
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	app.ServeHTTP(w, r)

	fmt.Println(w.Header().Get("X-API"))
	fmt.Println(w.Body.String())
	// Output:
	// true
	// data
}

func ExampleApp_Mount() {
	app := mux.New()

	v1 := app.Mount("/v1")
	v1.Get("/users", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		fmt.Fprint(w, "v1 users")
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	app.ServeHTTP(w, r)

	fmt.Println(w.Body.String())
	// Output: v1 users
}

func ExampleApp_Use() {
	app := mux.New()

	app.Use(func(handler mux.Handler) mux.Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("X-Custom", "active")
			return handler(ctx, w, r)
		}
	})

	app.Get("/ping", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		fmt.Fprint(w, "pong")
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ping", nil)
	app.ServeHTTP(w, r)

	fmt.Println(w.Header().Get("X-Custom"))
	fmt.Println(w.Body.String())
	// Output:
	// active
	// pong
}

// ————————————————————————————————————————————————————————————————————
// Option examples
// ————————————————————————————————————————————————————————————————————

func ExampleWithMiddleware() {
	// WithMiddleware auto-categorizes by function name:
	// CORS → global, Logger/Errors → route-level, Panics → outermost route-level.
	app := mux.New(
		mux.WithMiddleware(
			middleware.CORS([]string{"*"}),
			middleware.Logger(slog.Default()),
			middleware.Errors(slog.Default()),
			middleware.Panics(),
		),
	)

	_ = app
	fmt.Println("middleware auto-categorized")
	// Output: middleware auto-categorized
}

func ExampleWithStaticFS() {
	static := fstest.MapFS{
		"hello.txt": &fstest.MapFile{Data: []byte("static content")},
	}

	app := mux.New(
		mux.WithStaticFS(static, "/static/"),
	)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/static/hello.txt", nil)
	app.ServeHTTP(w, r)

	fmt.Println(w.Code)
	fmt.Println(w.Body.String())
	// Output:
	// 200
	// static content
}
