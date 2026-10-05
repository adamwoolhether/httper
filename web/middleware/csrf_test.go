package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adamwoolhether/httper/web/middleware"
)

func TestCSRF_Request(t *testing.T) {
	tests := map[string]struct {
		header     http.Header
		wantStatus int
	}{
		"cross-site fetch metadata": {
			header:     http.Header{"Sec-Fetch-Site": {"cross-site"}},
			wantStatus: http.StatusForbidden,
		},
		"untrusted origin": {
			header:     http.Header{"Origin": {"https://evil.example.com"}},
			wantStatus: http.StatusForbidden,
		},
		"trusted origin": {
			header:     http.Header{"Sec-Fetch-Site": {"cross-site"}, "Origin": {"https://app.example.com"}},
			wantStatus: http.StatusOK,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			called := false
			handler := middleware.CSRF("https://app.example.com")(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				called = true
				w.WriteHeader(http.StatusOK)
				return nil
			})

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "http://api.example.com/", nil)
			r.Header = tc.header

			if err := handler(r.Context(), w, r); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if wantCalled := tc.wantStatus == http.StatusOK; called != wantCalled {
				t.Fatalf("handler called = %v, want %v", called, wantCalled)
			}
			if tc.wantStatus == http.StatusOK {
				return
			}

			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("Content-Type = %q, want %q", ct, "application/json")
			}

			var body struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("body %q should be JSON: %v", w.Body.String(), err)
			}
			if body.Code != http.StatusForbidden || body.Message == "" {
				t.Fatalf("body = %+v, want code %d and a message", body, http.StatusForbidden)
			}
		})
	}
}

func TestCSRF_TrustedOrigin(t *testing.T) {
	tests := map[string]struct {
		origin    string
		wantPanic bool
	}{
		"scheme and host":  {origin: "https://app.example.com", wantPanic: false},
		"with port":        {origin: "https://app.example.com:8443", wantPanic: false},
		"wildcard host":    {origin: "https://*.example.com", wantPanic: false},
		"missing scheme":   {origin: "app.example.com", wantPanic: true},
		"trailing slash":   {origin: "https://app.example.com/", wantPanic: true},
		"with path":        {origin: "https://app.example.com/login", wantPanic: true},
		"with query":       {origin: "https://app.example.com?a=b", wantPanic: true},
		"missing host":     {origin: "https://", wantPanic: true},
		"unparsable value": {origin: "https://app example.com", wantPanic: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var rec any
			func() {
				defer func() { rec = recover() }()
				middleware.CSRF(tc.origin)
			}()

			if gotPanic := rec != nil; gotPanic != tc.wantPanic {
				t.Fatalf("CSRF(%q) panicked = %v (%v), want %v", tc.origin, gotPanic, rec, tc.wantPanic)
			}
		})
	}
}
