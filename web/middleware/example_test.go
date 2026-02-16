package middleware_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"

	"github.com/adamwoolhether/httper/web/errs"
	"github.com/adamwoolhether/httper/web/middleware"
)

// ————————————————————————————————————————————————————————————————————
// CORS examples
// ————————————————————————————————————————————————————————————————————

func ExampleCORS() {
	cors := middleware.CORS([]string{"https://example.com"})

	handler := cors(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		fmt.Fprint(w, "ok")
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://example.com")

	handler(context.Background(), w, r)

	fmt.Println(w.Header().Get("Access-Control-Allow-Origin"))
	fmt.Println(w.Body.String())
	// Output:
	// https://example.com
	// ok
}

func ExampleCheckOriginFunc() {
	check := middleware.CheckOriginFunc([]string{
		"https://example.com",
		"https://*.example.org",
	})

	fmt.Println(check("https://example.com"))
	fmt.Println(check("https://sub.example.org"))
	fmt.Println(check("https://other.com"))
	// Output:
	// true
	// true
	// false
}

// ————————————————————————————————————————————————————————————————————
// CSRF examples
// ————————————————————————————————————————————————————————————————————

func ExampleCSRF() {
	csrf := middleware.CSRF("https://example.com")

	handler := csrf(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		fmt.Fprint(w, "protected")
		return nil
	})

	// Safe methods bypass CSRF checks.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler(r.Context(), w, r)

	fmt.Println(w.Body.String())
	// Output: protected
}

// ————————————————————————————————————————————————————————————————————
// Request lifecycle middleware examples
// ————————————————————————————————————————————————————————————————————

func ExampleLogger() {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	logger := middleware.Logger(log)

	handler := logger(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		fmt.Fprint(w, "logged")
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	handler(r.Context(), w, r)

	fmt.Println(w.Body.String())
	// Output: logged
}

func ExampleErrors() {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	errMW := middleware.Errors(log)

	handler := errMW(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return errs.New(http.StatusNotFound, fmt.Errorf("item not found"))
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler(r.Context(), w, r)

	fmt.Println(w.Code)
	fmt.Println(w.Body.String())
	// Output:
	// 404
	// {"code":404,"message":"item not found"}
}

func ExamplePanics() {
	panics := middleware.Panics()

	handler := panics(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		fmt.Fprint(w, "safe")
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler(r.Context(), w, r)

	fmt.Println(w.Body.String())
	// Output: safe
}
