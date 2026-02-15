package middleware_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adamwoolhether/httper/web/errs"
	"github.com/adamwoolhether/httper/web/middleware"
)

func TestErrors_NoError(t *testing.T) {
	log := slog.New(slog.NewTextHandler(&discardWriter{}, nil))
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
	log := slog.New(slog.NewTextHandler(&discardWriter{}, nil))
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
}

func TestErrors_InternalError(t *testing.T) {
	log := slog.New(slog.NewTextHandler(&discardWriter{}, nil))
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
}

func TestErrors_FieldErrors(t *testing.T) {
	log := slog.New(slog.NewTextHandler(&discardWriter{}, nil))
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
	log := slog.New(slog.NewTextHandler(&discardWriter{}, nil))
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
}

// discardWriter is an io.Writer that discards all data.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

