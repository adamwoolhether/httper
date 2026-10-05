package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/adamwoolhether/httper/web/errs"
	"github.com/adamwoolhether/httper/web/middleware"
)

func TestErrors_NoError(t *testing.T) {
	log, _ := newTestLogger(t)
	mw := middleware.Errors(log)
	handler := mw(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := handler(r.Context(), w, r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestErrors_AppError(t *testing.T) {
	log, buf := newTestLogger(t)
	mw := middleware.Errors(log)
	handler := mw(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return errs.New(http.StatusBadRequest, fmt.Errorf("invalid input"))
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := handler(r.Context(), w, r); err != nil {
		t.Fatalf("unexpected error from middleware: %v", err)
	}

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var m map[string]any
	json.Unmarshal(w.Body.Bytes(), &m)
	if m["message"] != "invalid input" {
		t.Fatalf("message = %v, want %q", m["message"], "invalid input")
	}

	if !strings.Contains(buf.String(), "trace_id=") {
		t.Fatalf("expected traceID in error log output: %s", buf.String())
	}
}

func TestErrors_InternalError(t *testing.T) {
	log, buf := newTestLogger(t)
	mw := middleware.Errors(log)
	handler := mw(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return errs.NewInternal(fmt.Errorf("secret db error"))
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := handler(r.Context(), w, r); err != nil {
		t.Fatalf("unexpected error from middleware: %v", err)
	}

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var m map[string]any
	json.Unmarshal(w.Body.Bytes(), &m)
	// Internal errors should have their message obscured.
	if m["message"] != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("message = %v, want %q", m["message"], http.StatusText(http.StatusInternalServerError))
	}

	if !strings.Contains(buf.String(), "trace_id=") {
		t.Fatalf("expected traceID in error log output: %s", buf.String())
	}
}

func TestErrors_FieldErrors(t *testing.T) {
	log, _ := newTestLogger(t)
	mw := middleware.Errors(log)
	handler := mw(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return errs.NewFieldsError("email", fmt.Errorf("required"))
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := handler(r.Context(), w, r); err != nil {
		t.Fatalf("unexpected error from middleware: %v", err)
	}

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}

	var arr []map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &arr); err != nil {
		t.Fatalf("body should be JSON array: %v", err)
	}
	if len(arr) != 1 || arr[0]["field"] != "email" {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestErrors_PlainError(t *testing.T) {
	log, buf := newTestLogger(t)
	mw := middleware.Errors(log)
	handler := mw(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return fmt.Errorf("unexpected failure")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := handler(r.Context(), w, r); err != nil {
		t.Fatalf("unexpected error from middleware: %v", err)
	}

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var m map[string]any
	json.Unmarshal(w.Body.Bytes(), &m)
	// Plain error should be obscured just like internal errors.
	if m["message"] != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("message = %v, want %q", m["message"], http.StatusText(http.StatusInternalServerError))
	}

	if !strings.Contains(buf.String(), "trace_id=") {
		t.Fatalf("expected traceID in error log output: %s", buf.String())
	}
}

func TestErrors_SharedInternalError(t *testing.T) {
	const requests = 10
	const secret = "secret db error"

	log, buf := newTestLogger(t)
	shared := errs.NewInternal(errors.New(secret))
	handler := middleware.Errors(log)(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return shared
	})

	var wg sync.WaitGroup
	for range requests {
		wg.Go(func() {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			if err := handler(r.Context(), w, r); err != nil {
				t.Errorf("unexpected error from middleware: %v", err)
				return
			}

			var m map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
				t.Errorf("body should be JSON: %v", err)
				return
			}
			if m["message"] != http.StatusText(http.StatusInternalServerError) {
				t.Errorf("message = %v, want %q", m["message"], http.StatusText(http.StatusInternalServerError))
			}
		})
	}
	wg.Wait()

	if shared.Message != secret {
		t.Fatalf("shared error message = %q, want %q", shared.Message, secret)
	}
	if got := strings.Count(buf.String(), secret); got != requests {
		t.Fatalf("log has %d %q entries, want %d:\n%s", got, secret, requests, buf.String())
	}
}

func TestErrors_InternalHooks(t *testing.T) {
	tests := map[string]struct {
		err  error
		want bool
	}{
		"unknown error":       {err: errors.New("db down"), want: true},
		"internal error":      {err: errs.NewInternal(errors.New("db down")), want: true},
		"explicit 500":        {err: errs.New(http.StatusInternalServerError, errors.New("db down")), want: true},
		"client error":        {err: errs.New(http.StatusBadRequest, errors.New("bad input")), want: false},
		"field errors":        {err: errs.NewFieldsError("email", errors.New("required")), want: false},
		"panic error":         {err: &middleware.PanicError{Value: "boom"}, want: false},
		"wrapped panic error": {err: fmt.Errorf("handler: %w", &middleware.PanicError{Value: "boom"}), want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			log, _ := newTestLogger(t)

			var calls []string
			hook := func(name string) func(context.Context, error) {
				return func(ctx context.Context, err error) {
					if err != tc.err {
						t.Errorf("%s hook error = %v, want %v", name, err, tc.err)
					}
					calls = append(calls, name)
				}
			}

			handler := middleware.Errors(log, hook("first"), nil, hook("second"))(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				return tc.err
			})

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if err := handler(r.Context(), w, r); err != nil {
				t.Fatalf("unexpected error from middleware: %v", err)
			}

			want := ""
			if tc.want {
				want = "first,second"
			}
			if got := strings.Join(calls, ","); got != want {
				t.Fatalf("hook calls = %q, want %q", got, want)
			}
		})
	}
}
