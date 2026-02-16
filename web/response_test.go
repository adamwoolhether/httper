package web_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/errs"
)

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.Background()

	data := map[string]string{"status": "ok"}
	err := web.RespondJSON(ctx, w, http.StatusOK, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", ct, "application/json")
	}

	var m map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if m["status"] != "ok" {
		t.Fatalf("body status = %q, want %q", m["status"], "ok")
	}
}

func TestRespondJSON_NoContent(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.Background()

	err := web.RespondJSON(ctx, w, http.StatusNoContent, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("body should be empty, got %d bytes", w.Body.Len())
	}
}

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.Background()

	appErr := errs.New(http.StatusBadRequest, fmt.Errorf("bad input"))
	err := web.RespondError(ctx, w, appErr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m["message"] != "bad input" {
		t.Fatalf("message = %v, want %q", m["message"], "bad input")
	}
	if int(m["code"].(float64)) != http.StatusBadRequest {
		t.Fatalf("code = %v, want %d", m["code"], http.StatusBadRequest)
	}
}

func TestRedirect(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/old", nil)

	err := web.Redirect(w, r, "/new", http.StatusFound)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/new" {
		t.Fatalf("Location = %q, want %q", loc, "/new")
	}
}

func TestRedirect_InvalidCode(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/old", nil)

	tests := map[string]int{
		"200": http.StatusOK,
		"404": http.StatusNotFound,
		"500": http.StatusInternalServerError,
	}

	for name, code := range tests {
		t.Run(name, func(t *testing.T) {
			if err := web.Redirect(w, r, "/new", code); err == nil {
				t.Fatalf("expected error for non-3xx code %d", code)
			}
		})
	}
}
