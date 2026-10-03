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
	"strings"
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

// doGet builds a client from opts and sends one GET to u, expecting a 200.
func doGet(t *testing.T, u *url.URL, opts ...client.Option) error {
	t.Helper()

	c, err := client.Build(opts...)
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	req, err := c.Request(t.Context(), u, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	return c.Do(req, http.StatusOK)
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

func assertAttempts(t *testing.T, got *arrivals, want int) []time.Time {
	t.Helper()

	times, _ := got.snapshot()
	if len(times) != want {
		t.Fatalf("expected %d attempts, got %d", want, len(times))
	}

	return times
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
	// A 1ms backoff with a cap above each Retry-After: a gap of 1s can come only from the header.
	retryAfterBackoff := client.WithBackoff(time.Millisecond, 5*time.Second)

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
			opts:         []client.Option{client.WithRetry(1, retryAfterBackoff)},
			respond:      statuses("1", http.StatusTooManyRequests, http.StatusOK),
			wantStatus:   http.StatusOK,
			wantAttempts: 2,
			minGap:       time.Second,
		},
		"Retry-After as a date": {
			opts: []client.Option{client.WithRetry(1, retryAfterBackoff)},
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

			assertStatus(t, doGet(t, u, tc.opts...), tc.wantStatus)

			times := assertAttempts(t, got, tc.wantAttempts)
			for i := 1; i < len(times); i++ {
				if gap := times[i].Sub(times[i-1]); gap < tc.minGap {
					t.Errorf("attempt %d came %v after the previous one, want at least %v", i, gap, tc.minGap)
				}
			}
		})
	}
}

func TestClient_Retry_GivesUpOnLongWait(t *testing.T) {
	tests := map[string]struct {
		opts       []client.Option
		retryAfter string
	}{
		"Retry-After past the deadline": {
			opts:       []client.Option{client.WithTimeout(2 * time.Second), client.WithRetry(3)},
			retryAfter: "10",
		},
		"Retry-After above the backoff cap": {
			opts:       []client.Option{client.WithRetry(3, client.WithBackoff(time.Millisecond, time.Second))},
			retryAfter: "2",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			u, got := retryServer(t, statuses(tc.retryAfter, http.StatusTooManyRequests))

			start := time.Now()
			assertStatus(t, doGet(t, u, tc.opts...), http.StatusTooManyRequests)

			if elapsed := time.Since(start); elapsed > time.Second {
				t.Errorf("expected the 429 at once, took %v", elapsed)
			}
			assertAttempts(t, got, 1)
		})
	}
}

func TestClient_Retry_CanceledDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	u, got := retryServer(t, func(w http.ResponseWriter, attempt int) {
		time.AfterFunc(50*time.Millisecond, cancel)
		statuses("3", http.StatusServiceUnavailable)(w, attempt)
	})

	c, err := client.Build(client.WithRetry(3))
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	req, err := c.Request(ctx, u, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	start := time.Now()
	err = c.Do(req, http.StatusOK)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("expected the wait to end at cancel, took %v", elapsed)
	}
	assertAttempts(t, got, 1)
}

func TestClient_Retry_Body(t *testing.T) {
	payload := make([]byte, 256)
	for i := range payload {
		payload[i] = byte(i)
	}

	tests := map[string]struct {
		body         io.Reader
		wantStatus   int
		wantAttempts int
	}{
		"replays through GetBody": {body: bytes.NewReader(payload), wantStatus: http.StatusOK, wantAttempts: 2},
		"no GetBody sends once":   {body: io.NopCloser(bytes.NewReader(payload)), wantStatus: http.StatusServiceUnavailable, wantAttempts: 1},
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

			assertStatus(t, c.Do(req, http.StatusOK), tc.wantStatus)

			assertAttempts(t, got, tc.wantAttempts)
			_, bodies := got.snapshot()
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

	assertStatus(t, doGet(t, u, client.WithThrottle(5, 1), client.WithRetry(2, fastBackoff)), http.StatusOK)

	// 5 rps spaces tokens 200ms apart; the 1ms backoff alone would not.
	const minGap = 150 * time.Millisecond
	times := assertAttempts(t, got, 3)
	for i := 1; i < len(times); i++ {
		if gap := times[i].Sub(times[i-1]); gap < minGap {
			t.Errorf("attempt %d came %v after the previous one, want at least %v", i, gap, minGap)
		}
	}
}

func TestClient_Retry_SharedHTTPClient(t *testing.T) {
	u, got := retryServer(t, statuses("", http.StatusServiceUnavailable))

	shared := &http.Client{}
	if _, err := client.Build(client.WithClient(shared), client.WithRetry(3, fastBackoff)); err != nil {
		t.Fatalf("creating retrying client: %v", err)
	}

	assertStatus(t, doGet(t, u, client.WithClient(shared)), http.StatusServiceUnavailable)
	assertAttempts(t, got, 1)
}

// trackedBody records whether it was closed; Len reports what a reader left unread.
type trackedBody struct {
	*strings.Reader
	closed bool
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}

// flakyTransport answers the first send with a 503 that carries body, and every later send with a 200.
func flakyTransport(body io.ReadCloser, sends *atomic.Int32) http.RoundTripper {
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if sends.Add(1) == 1 {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{}, Body: body, Request: r}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: http.NoBody, Request: r}, nil
	})
}

func TestClient_Retry_DrainsDiscardedResponse(t *testing.T) {
	discarded := &trackedBody{Reader: strings.NewReader("service unavailable")}
	var sends atomic.Int32
	u := &url.URL{Scheme: "http", Host: "example.invalid"}

	assertStatus(t, doGet(t, u, client.WithTransport(flakyTransport(discarded, &sends)), client.WithRetry(1, fastBackoff)), http.StatusOK)

	if n := sends.Load(); n != 2 {
		t.Errorf("expected 2 sends, got %d", n)
	}
	if discarded.Len() != 0 {
		t.Errorf("expected the discarded body drained, %d bytes left", discarded.Len())
	}
	if !discarded.closed {
		t.Error("expected the discarded body closed")
	}
}

func TestClient_Retry_NilBodyResponse(t *testing.T) {
	var sends atomic.Int32
	u := &url.URL{Scheme: "http", Host: "example.invalid"}

	assertStatus(t, doGet(t, u, client.WithTransport(flakyTransport(nil, &sends)), client.WithRetry(1, fastBackoff)), http.StatusOK)

	if n := sends.Load(); n != 2 {
		t.Errorf("expected 2 sends, got %d", n)
	}
}

func TestClient_Retry_NilResponse(t *testing.T) {
	var sends atomic.Int32
	broken := roundTripFunc(func(*http.Request) (*http.Response, error) {
		sends.Add(1)
		return nil, nil
	})
	u := &url.URL{Scheme: "http", Host: "example.invalid"}

	if err := doGet(t, u, client.WithTransport(broken), client.WithRetry(1, fastBackoff)); err == nil {
		t.Fatal("expected error, got nil")
	}
	if n := sends.Load(); n != 1 {
		t.Errorf("expected 1 send, got %d", n)
	}
}

func assertSentOnceTransient(t *testing.T, serverURL string) {
	t.Helper()

	var sends atomic.Int32
	counting := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sends.Add(1)
		return http.DefaultTransport.RoundTrip(r)
	})

	u, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parsing server URL: %v", err)
	}

	err = doGet(t, u, client.WithTransport(counting), client.WithRetry(3, fastBackoff))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if n := sends.Load(); n != 1 {
		t.Errorf("expected 1 send, got %d", n)
	}
	if !client.IsRetryable(err) {
		t.Errorf("expected IsRetryable to report true for: %v", err)
	}
}

func TestClient_Retry_ClosedListener(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	ts.Close()

	assertSentOnceTransient(t, ts.URL)
}

func TestClient_Retry_DroppedConnection(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Errorf("hijacking connection: %v", err)
			return
		}
		if err := conn.Close(); err != nil {
			t.Errorf("closing hijacked connection: %v", err)
		}
	}))
	defer ts.Close()

	assertSentOnceTransient(t, ts.URL)
}

// timeoutError is an error whose Timeout method reports its value.
type timeoutError bool

func (e timeoutError) Error() string { return fmt.Sprintf("timeout: %t", bool(e)) }
func (e timeoutError) Timeout() bool { return bool(e) }

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
		"Timeout() true":       {err: fmt.Errorf("read: %w", timeoutError(true)), want: true},
		"Timeout() false":      {err: timeoutError(false), want: false},
		"round-trip url.Error": {err: &url.Error{Op: "Get", URL: "http://example.com", Err: io.EOF}, want: true},
		"net.OpError":          {err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, want: true},
		"unexpected EOF":       {err: fmt.Errorf("decoding body: %w", io.ErrUnexpectedEOF), want: true},
		"canceled":             {err: context.Canceled, want: false},
		"canceled round trip":  {err: &url.Error{Op: "Get", URL: "http://example.com", Err: context.Canceled}, want: false},
		"URL parse error":      {err: parseErr, want: false},
		"unclassified":         {err: errors.New("boom"), want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := client.IsRetryable(tc.err); got != tc.want {
				t.Errorf("IsRetryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
