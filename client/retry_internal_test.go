package client

import (
	"math"
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

func TestParseRetryAfter(t *testing.T) {
	const longest = time.Duration(math.MaxInt64/time.Second) * time.Second

	tests := map[string]struct {
		value  string
		want   time.Duration
		wantOK bool
	}{
		"seconds":             {value: "120", want: 120 * time.Second, wantOK: true},
		"past 32 bits":        {value: "4294967296", want: 4294967296 * time.Second, wantOK: true},
		"past 64 bits clamps": {value: "99999999999999999999", want: longest, wantOK: true},
		"overflow then junk":  {value: "99999999999999999999x", wantOK: false},
		"negative":            {value: "-1", wantOK: false},
		"empty":               {value: "", wantOK: false},
		"date in the past":    {value: "Mon, 02 Jan 2006 15:04:05 GMT", want: 0, wantOK: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := parseRetryAfter(tc.value)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("parseRetryAfter(%q) = %v, %v; want %v, %v", tc.value, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
