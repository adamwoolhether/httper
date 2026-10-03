package client

import (
	"net/http"
	"testing"
	"time"
)

func TestRetryConfig_WaitBackoff(t *testing.T) {
	const ms = time.Millisecond
	const huge = 1_000_000_000 * time.Second

	tests := map[string]struct {
		cfg      retryConfig
		backoffs []time.Duration
	}{
		"doubles up to the cap": {
			cfg:      retryConfig{baseWait: 100 * ms, maxWait: 300 * ms},
			backoffs: []time.Duration{100 * ms, 200 * ms, 300 * ms, 300 * ms},
		},
		"near-max durations do not overflow": {
			cfg:      retryConfig{baseWait: 5 * huge, maxWait: 8 * huge},
			backoffs: []time.Duration{5 * huge, 8 * huge, 8 * huge},
		},
	}

	resp := &http.Response{Header: http.Header{}}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			for retry, backoff := range tc.backoffs {
				for range 100 {
					if got := tc.cfg.wait(resp, retry); got < backoff/2 || got > backoff {
						t.Fatalf("retry %d: wait %v outside [%v, %v]", retry, got, backoff/2, backoff)
					}
				}
			}
		})
	}
}
