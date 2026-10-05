package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adamwoolhether/httper/web/errs"
	"github.com/adamwoolhether/httper/web/middleware"
)

func TestPanics_NoPanic(t *testing.T) {
	mw := middleware.Panics()
	handler := mw(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := handler(r.Context(), w, r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPanics_Recovery(t *testing.T) {
	mw := middleware.Panics()
	handler := mw(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		panic("something broke")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	err := handler(r.Context(), w, r)
	if err == nil {
		t.Fatal("expected error from recovered panic")
	}

	msg := err.Error()
	if !strings.Contains(msg, "PANIC") {
		t.Fatalf("error should contain PANIC, got: %s", msg)
	}
	if !strings.Contains(msg, "something broke") {
		t.Fatalf("error should contain panic value, got: %s", msg)
	}
	if !strings.Contains(msg, "TRACE") {
		t.Fatalf("error should contain TRACE, got: %s", msg)
	}
}

func TestPanics_ErrAbortHandler(t *testing.T) {
	hookCalled := false
	hook := func(ctx context.Context, err error) { hookCalled = true }
	handler := middleware.Panics(hook)(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		panic(http.ErrAbortHandler)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	var rec any
	func() {
		defer func() { rec = recover() }()
		_ = handler(r.Context(), w, r)
	}()

	if rec != http.ErrAbortHandler {
		t.Fatalf("panic value = %v, want http.ErrAbortHandler", rec)
	}
	if hookCalled {
		t.Fatal("onPanic hook ran for http.ErrAbortHandler")
	}
}

func TestPanics_Hooks(t *testing.T) {
	var calls []string
	var reported []error
	hook := func(name string) func(context.Context, error) {
		return func(ctx context.Context, err error) {
			calls = append(calls, name)
			reported = append(reported, err)
		}
	}

	handler := middleware.Panics(hook("first"), nil, hook("second"))(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		panic("boom")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	err := handler(r.Context(), w, r)

	if strings.Join(calls, ",") != "first,second" {
		t.Fatalf("hook calls = %v, want [first second]", calls)
	}
	for _, got := range reported {
		if !strings.Contains(got.Error(), "PANIC [boom]") || !strings.Contains(got.Error(), "panic_test.go") {
			t.Fatalf("hook error = %q, want the panic value and the panicking frame", got)
		}
	}

	appErr, ok := errors.AsType[*errs.Error](err)
	if !ok || !appErr.IsInternal() {
		t.Fatalf("err = %#v, want an internal *errs.Error", err)
	}
	if appErr.Message != reported[0].Error() {
		t.Fatalf("returned message = %q, want the reported panic error", appErr.Message)
	}
}
