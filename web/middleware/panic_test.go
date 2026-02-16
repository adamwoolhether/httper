package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
