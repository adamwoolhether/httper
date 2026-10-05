package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adamwoolhether/httper/web/middleware"
)

func okHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	w.WriteHeader(http.StatusOK)
	return nil
}

func TestCORS_AllowedOrigin(t *testing.T) {
	cors := middleware.CORS([]string{"https://example.com"})
	handler := cors(okHandler)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://example.com")

	if err := handler(r.Context(), w, r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := map[string]string{
		"Access-Control-Allow-Origin":      "https://example.com",
		"Vary":                             "Origin",
		"Access-Control-Allow-Methods":     "GET, OPTIONS, PUT, POST, PATCH, DELETE",
		"Access-Control-Allow-Credentials": "true",
		"Access-Control-Max-Age":           "86400",
	}

	for header, want := range checks {
		if got := w.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}

	// Check that Allow-Headers has the defaults.
	ah := w.Header().Get("Access-Control-Allow-Headers")
	if ah == "" {
		t.Fatal("Access-Control-Allow-Headers should be set")
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	cors := middleware.CORS([]string{"https://allowed.com"})
	handler := cors(okHandler)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://evil.com")

	if err := handler(r.Context(), w, r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}

	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("body should be JSON: %v", err)
	}
}

func TestCORS_NoOrigin(t *testing.T) {
	called := false
	inner := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		called = true
		w.WriteHeader(http.StatusOK)
		return nil
	}

	cors := middleware.CORS([]string{"https://example.com"})
	handler := cors(inner)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	// No Origin header set.

	if err := handler(r.Context(), w, r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !called {
		t.Fatal("handler should be called directly when no Origin header")
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS headers should not be set when no Origin")
	}
}

func TestCORS_Preflight(t *testing.T) {
	cors := middleware.CORS([]string{"https://example.com"})
	handler := cors(okHandler)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodOptions, "/", nil)
	r.Header.Set("Origin", "https://example.com")

	if err := handler(r.Context(), w, r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestCORS_AllowAll(t *testing.T) {
	tests := map[string]struct {
		allowed []string
		method  string
	}{
		"star":           {allowed: []string{"*"}, method: http.MethodGet},
		"star preflight": {allowed: []string{"*"}, method: http.MethodOptions},
		"star in list":   {allowed: []string{"https://a.com, *"}, method: http.MethodGet},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := middleware.CORS(tc.allowed)(okHandler)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, "/", nil)
			r.Header.Set("Origin", "https://any.com")

			if err := handler(r.Context(), w, r); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
			}
			if got, ok := w.Header()["Access-Control-Allow-Credentials"]; ok {
				t.Errorf("Access-Control-Allow-Credentials = %q, want header absent", got)
			}
		})
	}
}

func TestCORS_WildcardOriginReflected(t *testing.T) {
	handler := middleware.CORS([]string{"https://*.example.com"})(okHandler)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://api.example.com")

	if err := handler(r.Context(), w, r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://api.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://api.example.com")
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want %q", got, "true")
	}
}

func TestCORS_DefaultHeaders(t *testing.T) {
	cors := middleware.CORS([]string{"*"})
	handler := cors(okHandler)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://any.com")

	handler(r.Context(), w, r)

	ah := w.Header().Get("Access-Control-Allow-Headers")
	for _, h := range []string{"Authorization", "Content-Type", "Accept", "X-Requested-With", "Cache-Control"} {
		if !containsSubstring(ah, h) {
			t.Errorf("Allow-Headers %q missing %q", ah, h)
		}
	}
}

func TestCORS_CustomHeaders(t *testing.T) {
	cors := middleware.CORS([]string{"*"}, "X-Custom")
	handler := cors(okHandler)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://any.com")

	handler(r.Context(), w, r)

	ah := w.Header().Get("Access-Control-Allow-Headers")
	if ah != "X-Custom" {
		t.Fatalf("Allow-Headers = %q, want %q", ah, "X-Custom")
	}
}

func TestCheckOriginFunc(t *testing.T) {
	tests := map[string]struct {
		allowed []string
		origin  string
		want    bool
	}{
		"exact match": {
			allowed: []string{"https://example.com"},
			origin:  "https://example.com",
			want:    true,
		},
		"no match": {
			allowed: []string{"https://example.com"},
			origin:  "https://other.com",
			want:    false,
		},
		"wildcard pattern": {
			allowed: []string{"https://*.example.com"},
			origin:  "https://sub.example.com",
			want:    true,
		},
		"star allow-all": {
			allowed: []string{"*"},
			origin:  "https://anything.com",
			want:    true,
		},
		"comma-separated": {
			allowed: []string{"https://a.com,https://b.com"},
			origin:  "https://b.com",
			want:    true,
		},
		"comma-separated with spaces": {
			allowed: []string{"https://a.com, https://b.com "},
			origin:  "https://b.com",
			want:    true,
		},
		"star in comma-separated list": {
			allowed: []string{"https://a.com, *"},
			origin:  "https://anything.com",
			want:    true,
		},
		"wildcard matches one label only": {
			allowed: []string{"https://*.example.com"},
			origin:  "https://a.b.example.com",
			want:    false,
		},
		"wildcard needs a label": {
			allowed: []string{"https://*.example.com"},
			origin:  "https://.example.com",
			want:    false,
		},
		"wildcard excludes apex": {
			allowed: []string{"https://*.example.com"},
			origin:  "https://example.com",
			want:    false,
		},
		"wildcard keeps port": {
			allowed: []string{"https://*.example.com"},
			origin:  "https://api.example.com:8443",
			want:    false,
		},
		"wildcard keeps scheme": {
			allowed: []string{"https://*.example.com"},
			origin:  "http://api.example.com",
			want:    false,
		},
		"wildcard rejects suffix host": {
			allowed: []string{"https://*.example.com"},
			origin:  "https://api.example.com.evil.com",
			want:    false,
		},
		"wildcard with port": {
			allowed: []string{"https://*.example.com:8443"},
			origin:  "https://api.example.com:8443",
			want:    true,
		},
		"wildcard port": {
			allowed: []string{"http://localhost:*"},
			origin:  "http://localhost:3000",
			want:    true,
		},
		"wildcard host matches single-label host": {
			allowed: []string{"https://*"},
			origin:  "https://localhost",
			want:    true,
		},
		"wildcard host rejects dotted host": {
			allowed: []string{"https://*"},
			origin:  "https://evil.com",
			want:    false,
		},
		"multiple wildcards match nothing": {
			allowed: []string{"https://*.*.example.com"},
			origin:  "https://a.b.example.com",
			want:    false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fn := middleware.CheckOriginFunc(tc.allowed)
			if got := fn(tc.origin); got != tc.want {
				t.Fatalf("CheckOriginFunc(%v)(%q) = %v, want %v", tc.allowed, tc.origin, got, tc.want)
			}
		})
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
