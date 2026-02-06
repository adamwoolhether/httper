package throttle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
