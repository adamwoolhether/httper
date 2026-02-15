package middleware_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adamwoolhether/httper/web/middleware"
	"github.com/adamwoolhether/httper/web/mux"
)

func TestLogger(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))

	mw := middleware.Logger(log)
	handler := mw(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/hello", nil)
	r.RemoteAddr = "127.0.0.1:1234"

	handler(r.Context(), w, r)

	output := buf.String()
	if !strings.Contains(output, "request started") {
		t.Fatalf("expected 'request started' in log output: %s", output)
	}
	if !strings.Contains(output, "request completed") {
		t.Fatalf("expected 'request completed' in log output: %s", output)
	}
	if !strings.Contains(output, "GET") {
		t.Fatalf("expected method in log output: %s", output)
	}
	if !strings.Contains(output, "/hello") {
		t.Fatalf("expected path in log output: %s", output)
	}
	if !strings.Contains(output, "127.0.0.1:1234") {
		t.Fatalf("expected remoteaddr in log output: %s", output)
	}
}

func TestLogger_WithQuery(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))

	mw := middleware.Logger(log)
	handler := mw(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/search?q=test", nil)

	handler(r.Context(), w, r)

	output := buf.String()
	if !strings.Contains(output, "q=test") {
		t.Fatalf("expected query string in log output: %s", output)
	}
}

func TestLogger_StatusCode(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))

	mw := middleware.Logger(log)
	handler := mw(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		mux.SetStatusCode(ctx, http.StatusCreated)
		w.WriteHeader(http.StatusCreated)
		return nil
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/items", nil)

	handler(r.Context(), w, r)

	output := buf.String()
	if !strings.Contains(output, "statusCode") {
		t.Fatalf("expected statusCode in log output: %s", output)
	}
	if !strings.Contains(output, "since") {
		t.Fatalf("expected since in log output: %s", output)
	}
}
