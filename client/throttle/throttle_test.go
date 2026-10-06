package throttle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewRoundTripper_Validation(t *testing.T) {
	testCases := []struct {
		name   string
		rps    int
		burst  int
		expErr error
	}{
		{
			name:   "Invalid RPS (zero)",
			rps:    0,
			burst:  10,
			expErr: ErrMustNotBeZero,
		},
		{
			name:   "Invalid RPS (negative)",
			rps:    -5,
			burst:  10,
			expErr: ErrMustNotBeZero,
		},
		{
			name:   "Invalid Burst (zero)",
			rps:    10,
			burst:  0,
			expErr: ErrMustNotBeZero,
		},
		{
			name:   "Invalid Burst (negative)",
			rps:    10,
			burst:  -5,
			expErr: ErrMustNotBeZero,
		},
		{
			name:  "Valid input",
			rps:   10,
			burst: 20,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := NewRoundTripper(tc.rps, tc.burst, func() *slog.Logger { return nil }, http.DefaultTransport)

			if tc.expErr != nil {
				if !errors.Is(err, tc.expErr) {
					t.Errorf("exp err %v; got: %v", tc.expErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("exp nil err, got: %v", err)
				}

				if rt == nil {
					t.Error("exp non-nil RoundTripper")
				}
			}
		})
	}
}

func TestThrottleRoundTripper_Behavior(t *testing.T) {
	checkContextDeadlineWrapped := func(t *testing.T, err error, caseName string) {
		if err == nil {
			t.Errorf("%s should have returned an error", caseName)
		}
		if !errors.Is(err, ErrWaitingFailed) {
			t.Errorf("%s should have returned ErrWaitingFailed, got: %v", caseName, err)
		}
	}
	checkContextCancelledOrDeadline := func(t *testing.T, err error, caseName string) {
		if err == nil {
			t.Errorf("%s should have returned an error", caseName)
		}
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Errorf("%s should have returned context.DeadlineExceeded or context.Canceled, got %v", caseName, err)
		}
		if !errors.Is(err, ErrContextEnded) {
			t.Errorf("%s should have returned ErrContextEnded, got: %v", caseName, err)
		}
	}
	checkFast := func(t *testing.T, duration time.Duration, threshold time.Duration, caseName string) {
		if duration > threshold {
			t.Errorf("[%s] should be fast (< %v); but took %v", caseName, threshold, duration)
		}
	}
	checkSlowedDown := func(t *testing.T, duration time.Duration, minThreshold time.Duration, caseName string) {
		if duration < minThreshold {
			t.Errorf("[%s] execution should be slowed down by throttle (>= %v), but took %v", caseName, minThreshold, duration)
		}
	}

	testCases := []struct {
		name             string
		rps              int
		burst            int
		numRequests      int
		reqTimeout       time.Duration
		overallTimeout   time.Duration
		serverDelay      time.Duration // Simulate server processing time
		cancelContextIdx int           // Index of request to pre-cancel context for (-1 means none)
		expectReqErrs    int
		errorCheck       func(t *testing.T, err error, caseName string)
		timingCheck      func(t *testing.T, duration time.Duration, caseName string)
	}{
		{
			name:             "High Limits - Concurrent Load",
			rps:              10000,
			burst:            100,
			numRequests:      50,
			reqTimeout:       0,
			overallTimeout:   1 * time.Second,
			serverDelay:      2 * time.Millisecond,
			cancelContextIdx: -1,
			expectReqErrs:    0,
			errorCheck:       nil,
			timingCheck: func(t *testing.T, duration time.Duration, caseName string) {
				checkFast(t, duration, 200*time.Millisecond, caseName)
			},
		},
		{
			name:             "Low Limit - Exceed Burst & Timeout Waiting",
			rps:              5,
			burst:            2,
			numRequests:      5, // 2 use burst, 3rd waits >50ms, 4th waits >50ms, 5th waits >50ms
			reqTimeout:       50 * time.Millisecond,
			overallTimeout:   1 * time.Second,
			serverDelay:      1 * time.Millisecond,
			cancelContextIdx: -1,
			expectReqErrs:    3,
			errorCheck:       checkContextDeadlineWrapped,
			timingCheck:      nil,
		},
		{
			name:             "Low Limit - Exceed Burst - Succeed Waiting",
			rps:              10,
			burst:            5,
			numRequests:      8, // 5 use burst, 3 need to wait (up to 100ms each)
			reqTimeout:       500 * time.Millisecond,
			overallTimeout:   1 * time.Second,
			serverDelay:      2 * time.Millisecond,
			cancelContextIdx: -1,
			expectReqErrs:    0,
			errorCheck:       nil,
			timingCheck: func(t *testing.T, duration time.Duration, caseName string) {
				// Expect duration >= time for rate-limited calls
				// (8-5 calls) / 10 RPS = 0.3 seconds
				minDuration := time.Duration(float64(time.Second) * float64(8-5) / float64(10))
				checkSlowedDown(t, duration, minDuration, caseName)
			},
		},
		{
			name:             "Low Limit - Within Burst",
			rps:              5,
			burst:            5,
			numRequests:      5,
			reqTimeout:       0,
			overallTimeout:   500 * time.Millisecond,
			serverDelay:      2 * time.Millisecond,
			cancelContextIdx: -1,
			expectReqErrs:    0,
			errorCheck:       nil,
			timingCheck: func(t *testing.T, duration time.Duration, caseName string) {
				checkFast(t, duration, 100*time.Millisecond, caseName)
			},
		},
		{
			name:             "Pre-Cancelled Context Fails Early",
			rps:              20,
			burst:            10,
			numRequests:      1,
			reqTimeout:       1 * time.Second,
			overallTimeout:   500 * time.Millisecond,
			serverDelay:      5 * time.Millisecond,
			cancelContextIdx: 0,
			expectReqErrs:    1,
			errorCheck:       checkContextCancelledOrDeadline,
			timingCheck: func(t *testing.T, duration time.Duration, caseName string) {
				checkFast(t, duration, 50*time.Millisecond, caseName)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var callCount int32

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.serverDelay > 0 {
					time.Sleep(tc.serverDelay)
				}

				atomic.AddInt32(&callCount, 1)

				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte(`{"status":"ok"}`))
				if err != nil {
					t.Fatal(err)
				}
			}))
			defer server.Close()

			rt, err := NewRoundTripper(tc.rps, tc.burst, func() *slog.Logger { return nil }, http.DefaultTransport)
			if err != nil {
				t.Fatal(err)
			}

			client := &http.Client{
				Transport: rt,
			}

			var wg sync.WaitGroup
			errs := make([]error, tc.numRequests)
			overallCtx, overallCancel := context.WithTimeout(context.Background(), tc.overallTimeout)
			defer overallCancel()

			start := time.Now()

			// launch requests concurrently
			for i := 0; i < tc.numRequests; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()

					var reqCtx context.Context
					var reqCancel context.CancelFunc = func() {}

					if idx == tc.cancelContextIdx {
						reqCtx, reqCancel = context.WithCancel(overallCtx)
						reqCancel()
					} else if tc.reqTimeout > 0 {
						reqCtx, reqCancel = context.WithTimeout(overallCtx, tc.reqTimeout)
					} else {
						reqCtx = overallCtx
					}
					defer reqCancel()

					req, reqErr := http.NewRequestWithContext(reqCtx, http.MethodGet, server.URL, nil)
					if reqErr != nil {
						errs[idx] = fmt.Errorf("failed create req %d: %w", idx, reqErr)
						return
					}

					resp, doErr := client.Do(req)
					errs[idx] = doErr

					if doErr == nil && resp != nil && resp.Body != nil {
						resp.Body.Close()
					} else if doErr == nil && resp == nil {
						errs[idx] = fmt.Errorf("request %d: got nil response and nil error", idx)
					}
				}(i)
			}

			wg.Wait()
			duration := time.Since(start)

			failedRequests := 0
			for i, err := range errs {
				if err != nil {
					failedRequests++
					t.Logf("Request %d failed with: %v", i, err)
					if tc.errorCheck != nil {
						tc.errorCheck(t, err, tc.name)
					}
				}
			}

			if tc.expectReqErrs != failedRequests {
				t.Errorf("expected %d failed requests; got %d", tc.expectReqErrs, failedRequests)
			}

			expectedServerCalls := int32(tc.numRequests - failedRequests)
			// Adjust if context was cancelled before the call could even reach the server
			if tc.cancelContextIdx != -1 && tc.expectReqErrs > 0 && failedRequests > 0 {
				// If the specific pre-cancelled request indeed failed, it likely didn't hit the server
				// This logic might need refinement based on exactly *when* the pre-cancel check fails
				// vs when the server count increments. Assume for now pre-cancel doesn't hit server.
				if containsDirectContextError(errs) {
					expectedServerCalls = int32(tc.numRequests - tc.expectReqErrs)
				}
			}
			if expectedServerCalls != atomic.LoadInt32(&callCount) {
				t.Errorf("[%s] Unexpected number of calls reached the server; exp %d, got %d", tc.name, expectedServerCalls, atomic.LoadInt32(&callCount))
			}

			if tc.timingCheck != nil {
				tc.timingCheck(t, duration, tc.name)
			}
		})
	}
}

func TestThrottleRoundTripper_LoggerTakesOneToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	rt, err := NewRoundTripper(1, 3, func() *slog.Logger { return logger }, http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: rt}

	send := func(timeout time.Duration) error {
		ctx, cancel := context.WithTimeout(t.Context(), timeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		return resp.Body.Close()
	}

	exhausted := func() int { return strings.Count(logs.String(), "throttle tokens exhausted") }

	// At 1 rps a token takes 1s to refill, so a request that needs a wait fails its short timeout.
	for i := range 3 {
		if err := send(100 * time.Millisecond); err != nil {
			t.Fatalf("request %d within the burst of 3: %v", i, err)
		}
	}
	if n := exhausted(); n != 0 {
		t.Errorf("expected no exhausted log within the burst, got %d", n)
	}

	if err := send(10 * time.Millisecond); err == nil {
		t.Fatal("expected request 4 to fail waiting for a token, got nil")
	}
	if n := exhausted(); n != 1 {
		t.Errorf("expected 1 exhausted log after the burst, got %d", n)
	}
}

func TestNewRoundTripperEvery_Validation(t *testing.T) {
	tests := map[string]struct {
		interval time.Duration
		burst    int
	}{
		"zero interval":     {interval: 0, burst: 1},
		"negative interval": {interval: -time.Second, burst: 1},
		"zero burst":        {interval: time.Second, burst: 0},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewRoundTripperEvery(tc.interval, tc.burst, func() *slog.Logger { return nil }, http.DefaultTransport)
			if !errors.Is(err, ErrMustNotBeZero) {
				t.Errorf("expected ErrMustNotBeZero, got: %v", err)
			}
		})
	}
}

func TestThrottleRoundTripper_LogsRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	tests := map[string]struct {
		newRT    func(logFn func() *slog.Logger) (http.RoundTripper, error)
		wantRate string
	}{
		"rps": {
			newRT: func(logFn func() *slog.Logger) (http.RoundTripper, error) {
				return NewRoundTripper(2, 1, logFn, http.DefaultTransport)
			},
			wantRate: "rate=2 ",
		},
		"interval": {
			newRT: func(logFn func() *slog.Logger) (http.RoundTripper, error) {
				return NewRoundTripperEvery(400*time.Millisecond, 1, logFn, http.DefaultTransport)
			},
			wantRate: "rate=2.5 ",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			rt, err := tc.newRT(func() *slog.Logger { return logger })
			if err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Transport: rt}

			send := func(ctx context.Context) error {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
				if err != nil {
					return err
				}
				resp, err := client.Do(req)
				if err != nil {
					return err
				}
				return resp.Body.Close()
			}

			if err := send(t.Context()); err != nil {
				t.Fatalf("first request spends the burst token: %v", err)
			}

			// The second request finds no token, so it logs "tokens exhausted"; its failed wait logs nothing more.
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
			defer cancel()
			if err := send(ctx); err == nil {
				t.Fatal("expected the second request to fail waiting for a token, got nil")
			}

			if n := strings.Count(logs.String(), tc.wantRate); n != 1 {
				t.Errorf("expected 1 log line with %q, got %d:\n%s", tc.wantRate, n, logs.String())
			}
		})
	}
}

func TestThrottleRoundTripper_ClosesBodyOnError(t *testing.T) {
	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	// An hour per token makes the wait exceed this deadline, so the limiter fails it at once.
	beforeToken, cancelBeforeToken := context.WithTimeout(t.Context(), time.Minute)
	defer cancelBeforeToken()

	tests := map[string]struct {
		ctx        context.Context
		spendToken bool
		wantErr    error
	}{
		"early context check":     {ctx: canceled, wantErr: ErrContextEnded},
		"failed wait":             {ctx: beforeToken, spendToken: true, wantErr: ErrWaitingFailed},
		"post-wait context check": {ctx: &endsAfterFirstCheck{Context: t.Context()}, wantErr: ErrContextEnded},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var sent int
			next := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				sent++
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: r}, nil
			})
			rt, err := NewRoundTripperEvery(time.Hour, 1, func() *slog.Logger { return nil }, next)
			if err != nil {
				t.Fatal(err)
			}

			if tc.spendToken {
				spend, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.invalid", nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := rt.RoundTrip(spend); err != nil {
					t.Fatalf("spending the burst token: %v", err)
				}
				sent = 0
			}

			body := &closeCounter{Reader: strings.NewReader("payload")}
			req, err := http.NewRequestWithContext(tc.ctx, http.MethodPost, "http://example.invalid", body)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := rt.RoundTrip(req); !errors.Is(err, tc.wantErr) {
				t.Errorf("expected %v, got: %v", tc.wantErr, err)
			}
			if sent != 0 {
				t.Errorf("expected no request sent to next, got %d", sent)
			}
			if body.closes != 1 {
				t.Errorf("expected 1 body close, got %d", body.closes)
			}
		})
	}
}

func TestThrottleRoundTripper_PassesRequestOn(t *testing.T) {
	var sent []*http.Request
	next := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sent = append(sent, r)
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: r}, nil
	})
	rt, err := NewRoundTripperEvery(time.Hour, 1, func() *slog.Logger { return nil }, next)
	if err != nil {
		t.Fatal(err)
	}

	body := &closeCounter{Reader: strings.NewReader("payload")}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.invalid", body)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(sent) != 1 || sent[0] != req {
		t.Errorf("expected next to receive the request unchanged, got %v", sent)
	}
	if body.closes != 0 {
		t.Errorf("expected the body left for next to close, got %d closes", body.closes)
	}
}

func TestThrottleRoundTripper_LogsAtDebug(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	rt, err := NewRoundTripperEvery(200*time.Millisecond, 1, func() *slog.Logger { return logger }, http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	client := &http.Client{Transport: rt}

	send := func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		return resp.Body.Close()
	}

	if err := send(t.Context()); err != nil {
		t.Fatalf("first request spends the burst token: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err := send(ctx); !errors.Is(err, ErrWaitingFailed) {
		t.Fatalf("expected the second request to fail its wait, got: %v", err)
	}
	if strings.Contains(logs.String(), "throttle wait complete") {
		t.Errorf("expected no wait complete line after a failed wait:\n%s", logs.String())
	}

	if err := send(t.Context()); err != nil {
		t.Fatalf("third request waits for a token: %v", err)
	}
	if n := strings.Count(logs.String(), "throttle wait complete"); n != 1 {
		t.Errorf("expected 1 wait complete line after a successful wait, got %d:\n%s", n, logs.String())
	}

	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 log lines, got %d:\n%s", len(lines), logs.String())
	}
	for _, line := range lines {
		if !strings.Contains(line, "level=DEBUG") {
			t.Errorf("expected only debug lines, got: %s", line)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// closeCounter is a request body that counts its Close calls.
type closeCounter struct {
	io.Reader
	closes int
}

func (b *closeCounter) Close() error {
	b.closes++
	return nil
}

// endsAfterFirstCheck reports no error to its first Err call and context.Canceled to every later one.
// A fresh limiter has a token and rate.Limiter.Wait never calls Err on that path, so the request clears
// the early check and the wait, then ends at the post-wait check.
type endsAfterFirstCheck struct {
	context.Context
	checks int
}

func (c *endsAfterFirstCheck) Err() error {
	c.checks++
	if c.checks == 1 {
		return nil
	}
	return context.Canceled
}

func containsDirectContextError(errs []error) bool {
	for _, err := range errs {
		if err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			if err.Error() == fmt.Errorf("throttle context ended early: %w", err).Error() || err.Error() == fmt.Errorf("throttle context ended post-wait: %w", err).Error() {
				return true
			}
			// Handle cases where the error might not be wrapped by the throttle message if it happens *very* early
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				// Crude check if it doesn't contain "throttle wait"
				if !errors.Is(err, fmt.Errorf("throttle wait: %w", context.Canceled)) && !errors.Is(err, fmt.Errorf("throttle wait: %w", context.DeadlineExceeded)) {
					return true
				}
			}

		}
	}
	return false
}
