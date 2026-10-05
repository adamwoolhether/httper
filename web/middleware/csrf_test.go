package middleware_test

import (
	"testing"

	"github.com/adamwoolhether/httper/web/middleware"
)

func TestCSRF_TrustedOrigin(t *testing.T) {
	tests := map[string]struct {
		origin    string
		wantPanic bool
	}{
		"scheme and host":  {origin: "https://app.example.com", wantPanic: false},
		"with port":        {origin: "https://app.example.com:8443", wantPanic: false},
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
