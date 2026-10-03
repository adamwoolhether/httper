package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	defaultBaseWait = time.Second
	defaultMaxWait  = 30 * time.Second
)

type retryConfig struct {
	maxRetries int
	baseWait   time.Duration
	maxWait    time.Duration
}

// retryTransport is an http.RoundTripper that sends a request again after a 429 or 5xx response.
type retryTransport struct {
	retryConfig
	next http.RoundTripper
}

func (t retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	replayable := req.Body == nil || req.Body == http.NoBody || req.GetBody != nil

	resp, err := t.next.RoundTrip(req)
	for retry := range t.maxRetries {
		if err != nil || resp == nil || !replayable || !retryableStatus(resp.StatusCode) {
			break
		}

		wait := t.wait(resp, retry)
		if deadline, ok := ctx.Deadline(); wait > t.maxWait || (ok && time.Now().Add(wait).After(deadline)) {
			break
		}

		// http.Client accepts a nil Body from a custom transport (golang.org/issue/38095).
		// The capped drain keeps a short body's connection for reuse. A longer body or a drain
		// error costs only that connection, since the next attempt replaces resp.
		if resp.Body != nil {
			_, _ = io.CopyN(io.Discard, resp.Body, maxErrBodySize)
			_ = resp.Body.Close()
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}

		attempt, rewindErr := rewind(req)
		if rewindErr != nil {
			return nil, rewindErr
		}

		resp, err = t.next.RoundTrip(attempt)
	}

	return resp, err
}

func (c retryConfig) wait(resp *http.Response, retry int) time.Duration {
	if d, ok := parseRetryAfter(resp.Header.Get("Retry-After")); ok {
		return d
	}

	d := c.baseWait
	for range retry {
		if d > c.maxWait/2 {
			d = c.maxWait
			break
		}
		d *= 2
	}

	// Equal jitter: spreads clients out but keeps every wait at least half the backoff.
	return d/2 + rand.N(d/2+1)
}

func parseRetryAfter(v string) (time.Duration, bool) {
	// On ErrRange, ParseUint returns its maximum; the clamp keeps the product from overflowing.
	if secs, err := strconv.ParseUint(v, 10, 64); err == nil || errors.Is(err, strconv.ErrRange) {
		return time.Duration(min(secs, uint64(math.MaxInt64/time.Second))) * time.Second, true
	}

	if t, err := http.ParseTime(v); err == nil {
		return max(time.Until(t), 0), true
	}

	return 0, false
}

func rewind(req *http.Request) (*http.Request, error) {
	attempt := req.Clone(req.Context())
	if req.GetBody == nil {
		return attempt, nil
	}

	body, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("rewinding request body: %w", err)
	}
	attempt.Body = body

	return attempt, nil
}

func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || (code >= 500 && code < 600)
}

// IsRetryable reports whether err may be transient: a 429 or 5xx status, a timeout, or a transport failure with no response.
func IsRetryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return true
	}

	var statusErr *UnexpectedStatusError
	if errors.As(err, &statusErr) {
		return retryableStatus(statusErr.StatusCode)
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Op != "parse"
	}

	var opErr *net.OpError
	return errors.As(err, &opErr) || errors.Is(err, io.ErrUnexpectedEOF)
}
