package client_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adamwoolhether/httper/client"
)

var fastBackoff = client.WithBackoff(time.Millisecond, time.Millisecond)

// arrivals records each request a test server receives.
type arrivals struct {
	mu     sync.Mutex
	times  []time.Time
	bodies [][]byte
}

func (a *arrivals) record(r *http.Request) (int, error) {
	body, err := io.ReadAll(r.Body)

	a.mu.Lock()
	defer a.mu.Unlock()
	a.times = append(a.times, time.Now())
	a.bodies = append(a.bodies, body)

	return len(a.times) - 1, err
}

func (a *arrivals) snapshot() ([]time.Time, [][]byte) {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.times, a.bodies
}

// retryServer passes respond the 0-based index of each request it receives.
func retryServer(t *testing.T, respond func(w http.ResponseWriter, attempt int)) (*url.URL, *arrivals) {
	t.Helper()

	var a arrivals
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt, err := a.record(r)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		respond(w, attempt)
	}))
	t.Cleanup(ts.Close)

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	return u, &a
}

// statuses answers attempt i with codes[i], or the last code once codes run out.
func statuses(retryAfter string, codes ...int) func(http.ResponseWriter, int) {
	return func(w http.ResponseWriter, attempt int) {
		code := codes[min(attempt, len(codes)-1)]
		if code != http.StatusOK && retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(code)
	}
}

func assertStatus(t *testing.T, err error, want int) {
	t.Helper()

	if want == http.StatusOK {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		return
	}

	var statusErr *client.UnexpectedStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected *UnexpectedStatusError, got: %T: %v", err, err)
	}
	if statusErr.StatusCode != want {
		t.Errorf("expected status %d, got %d", want, statusErr.StatusCode)
	}
}

func TestClient_WithRetryValidation(t *testing.T) {
	tests := map[string]client.Option{
		"negative retries": client.WithRetry(-1),
		"zero base":        client.WithRetry(1, client.WithBackoff(0, time.Second)),
		"max below base":   client.WithRetry(1, client.WithBackoff(time.Second, time.Millisecond)),
	}

	for name, opt := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := client.Build(opt); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestClient_Retry(t *testing.T) {
	tests := map[string]struct {
		opts         []client.Option
		respond      func(http.ResponseWriter, int)
		wantStatus   int
		wantAttempts int
		minGap       time.Duration
	}{
		"429 then 200": {
			opts:         []client.Option{client.WithRetry(1, fastBackoff)},
			respond:      statuses("", http.StatusTooManyRequests, http.StatusOK),
			wantStatus:   http.StatusOK,
			wantAttempts: 2,
		},
		"Retry-After in seconds": {
			opts:         []client.Option{client.WithRetry(1, fastBackoff)},
			respond:      statuses("1", http.StatusTooManyRequests, http.StatusOK),
			wantStatus:   http.StatusOK,
			wantAttempts: 2,
			minGap:       time.Second,
		},
		"Retry-After as a date": {
			opts: []client.Option{client.WithRetry(1, fastBackoff)},
			respond: func(w http.ResponseWriter, attempt int) {
				// HTTP dates drop sub-second precision, so 2s ahead is at least 1s ahead.
				date := time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat)
				statuses(date, http.StatusTooManyRequests, http.StatusOK)(w, attempt)
			},
			wantStatus:   http.StatusOK,
			wantAttempts: 2,
			minGap:       time.Second,
		},
		"exhausted 5xx": {
			opts:         []client.Option{client.WithRetry(2, fastBackoff)},
			respond:      statuses("", http.StatusServiceUnavailable),
			wantStatus:   http.StatusServiceUnavailable,
			wantAttempts: 3,
		},
		"400 sends once": {
			opts:         []client.Option{client.WithRetry(3, fastBackoff)},
			respond:      statuses("", http.StatusBadRequest),
			wantStatus:   http.StatusBadRequest,
			wantAttempts: 1,
		},
		"maxRetries 0 sends once": {
			opts:         []client.Option{client.WithRetry(0)},
			respond:      statuses("", http.StatusServiceUnavailable),
			wantStatus:   http.StatusServiceUnavailable,
			wantAttempts: 1,
		},
		"no WithRetry sends once": {
			respond:      statuses("", http.StatusServiceUnavailable),
			wantStatus:   http.StatusServiceUnavailable,
			wantAttempts: 1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			u, got := retryServer(t, tc.respond)

			c, err := client.Build(tc.opts...)
			if err != nil {
				t.Fatalf("creating client: %v", err)
			}

			req, err := c.Request(t.Context(), u, http.MethodGet)
			if err != nil {
				t.Fatalf("creating request: %v", err)
			}

			assertStatus(t, c.Do(req, http.StatusOK), tc.wantStatus)

			times, _ := got.snapshot()
			if len(times) != tc.wantAttempts {
				t.Fatalf("expected %d attempts, got %d", tc.wantAttempts, len(times))
			}
			for i := 1; i < len(times); i++ {
				if gap := times[i].Sub(times[i-1]); gap < tc.minGap {
					t.Errorf("attempt %d came %v after the previous one, want at least %v", i, gap, tc.minGap)
				}
			}
		})
	}
}

func TestClient_Retry_RetryAfterPastDeadline(t *testing.T) {
	u, got := retryServer(t, statuses("60", http.StatusTooManyRequests))

	c, err := client.Build(client.WithTimeout(5*time.Second), client.WithRetry(3))
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	req, err := c.Request(t.Context(), u, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	start := time.Now()
	assertStatus(t, c.Do(req, http.StatusOK), http.StatusTooManyRequests)

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("expected the 429 at once, took %v", elapsed)
	}
	if times, _ := got.snapshot(); len(times) != 1 {
		t.Errorf("expected 1 attempt, got %d", len(times))
	}
}

func TestClient_Retry_Body(t *testing.T) {
	payload := make([]byte, 256)
	for i := range payload {
		payload[i] = byte(i)
	}

	tests := map[string]struct {
		body         io.Reader
		wantAttempts int
	}{
		"replays through GetBody": {body: bytes.NewReader(payload), wantAttempts: 2},
		"no GetBody sends once":   {body: io.NopCloser(bytes.NewReader(payload)), wantAttempts: 1},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			u, got := retryServer(t, statuses("", http.StatusServiceUnavailable, http.StatusOK))

			// On a reused connection http.Transport rewinds through GetBody itself; fresh
			// connections leave the retry's rewind as the only way to replay the body.
			fresh := &http.Transport{DisableKeepAlives: true}
			c, err := client.Build(client.WithTransport(fresh), client.WithRetry(1, fastBackoff))
			if err != nil {
				t.Fatalf("creating client: %v", err)
			}

			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, u.String(), tc.body)
			if err != nil {
				t.Fatalf("creating request: %v", err)
			}

			err = c.Do(req, http.StatusOK)

			_, bodies := got.snapshot()
			if len(bodies) != tc.wantAttempts {
				t.Fatalf("expected %d attempts, got %d (err: %v)", tc.wantAttempts, len(bodies), err)
			}
			for i, body := range bodies {
				if !bytes.Equal(body, payload) {
					t.Errorf("attempt %d body differs from payload: got %d bytes", i, len(body))
				}
			}
		})
	}
}

func TestClient_Retry_Throttled(t *testing.T) {
	u, got := retryServer(t, statuses("", http.StatusServiceUnavailable, http.StatusServiceUnavailable, http.StatusOK))

	c, err := client.Build(client.WithThrottle(5, 1), client.WithRetry(2, fastBackoff))
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	req, err := c.Request(t.Context(), u, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	assertStatus(t, c.Do(req, http.StatusOK), http.StatusOK)

	times, _ := got.snapshot()
	if len(times) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(times))
	}

	// 5 rps spaces tokens 200ms apart; the 1ms backoff alone would not.
	const minGap = 150 * time.Millisecond
	for i := 1; i < len(times); i++ {
		if gap := times[i].Sub(times[i-1]); gap < minGap {
			t.Errorf("attempt %d came %v after the previous one, want at least %v", i, gap, minGap)
		}
	}
}

func TestClient_Retry_TransportFailure(t *testing.T) {
	tests := map[string]func(t *testing.T) string{
		"closed listener": func(t *testing.T) string {
			ts := httptest.NewServer(http.NotFoundHandler())
			ts.Close()
			return ts.URL
		},
		"dropped connection": func(t *testing.T) string {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, _, err := http.NewResponseController(w).Hijack()
				if err != nil {
					t.Errorf("hijacking connection: %v", err)
					return
				}
				_ = conn.Close()
			}))
			t.Cleanup(ts.Close)
			return ts.URL
		},
	}

	for name, serverURL := range tests {
		t.Run(name, func(t *testing.T) {
			var sends atomic.Int32
			counting := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				sends.Add(1)
				return http.DefaultTransport.RoundTrip(r)
			})

			c, err := client.Build(client.WithTransport(counting), client.WithRetry(3, fastBackoff))
			if err != nil {
				t.Fatalf("creating client: %v", err)
			}

			u, err := url.Parse(serverURL(t))
			if err != nil {
				t.Fatalf("parsing server URL: %v", err)
			}

			req, err := c.Request(t.Context(), u, http.MethodGet)
			if err != nil {
				t.Fatalf("creating request: %v", err)
			}

			err = c.Do(req, http.StatusOK)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if n := sends.Load(); n != 1 {
				t.Errorf("expected 1 send, got %d", n)
			}
			if !client.IsRetryable(err) {
				t.Errorf("expected IsRetryable to report true for: %v", err)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	_, parseErr := client.Request(t.Context(), &url.URL{Scheme: "http", Host: "[::1"}, http.MethodGet)
	var urlErr *url.Error
	if !errors.As(parseErr, &urlErr) || urlErr.Op != "parse" {
		t.Fatalf("expected a URL parse error from Request, got: %v", parseErr)
	}

	tests := map[string]struct {
		err  error
		want bool
	}{
		"nil":                  {err: nil, want: false},
		"429":                  {err: &client.UnexpectedStatusError{StatusCode: http.StatusTooManyRequests}, want: true},
		"500":                  {err: &client.UnexpectedStatusError{StatusCode: http.StatusInternalServerError}, want: true},
		"wrapped 503":          {err: fmt.Errorf("exec: %w", &client.UnexpectedStatusError{StatusCode: http.StatusServiceUnavailable}), want: true},
		"400":                  {err: &client.UnexpectedStatusError{StatusCode: http.StatusBadRequest}, want: false},
		"404":                  {err: &client.UnexpectedStatusError{StatusCode: http.StatusNotFound}, want: false},
		"deadline exceeded":    {err: fmt.Errorf("exec: %w", context.DeadlineExceeded), want: true},
		"Timeout() true":       {err: &net.DNSError{Err: "timed out", IsTimeout: true}, want: true},
		"round-trip url.Error": {err: &url.Error{Op: "Get", URL: "http://example.com", Err: io.EOF}, want: true},
		"net.OpError":          {err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, want: true},
		"unexpected EOF":       {err: fmt.Errorf("decoding body: %w", io.ErrUnexpectedEOF), want: true},
		"canceled":             {err: context.Canceled, want: false},
		"canceled round trip":  {err: &url.Error{Op: "Get", URL: "http://example.com", Err: context.Canceled}, want: false},
		"URL parse error":      {err: parseErr, want: false},
		"unclassified":         {err: errors.New("boom"), want: false},
		"Timeout() false":      {err: &net.DNSError{Err: "no such host", IsNotFound: true}, want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := client.IsRetryable(tc.err); got != tc.want {
				t.Errorf("IsRetryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
