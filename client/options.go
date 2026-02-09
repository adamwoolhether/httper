package client

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/adamwoolhether/httper/client/throttle"
)

// Option is a functional option for configuring a [Client] via [Build].
type Option func(*options) error
type options struct {
	client            *http.Client
	rt                http.RoundTripper
	timeout           *time.Duration
	userAgent         string
	throttle          *throttle.Config
	noFollowRedirects bool
	logger            *slog.Logger
}

// WithClient replaces the default [http.Client] used by the [Client].
func WithClient(hc *http.Client) Option {
	return func(c *options) error {
		if hc == nil {
			return errors.New("client must not be nil")
		}
		c.client = hc
		return nil
	}
}

// WithTransport sets a custom [http.RoundTripper] as the base transport.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *options) error {
		if rt == nil {
			return errors.New("transport must not be nil")
		}
		c.rt = rt
		return nil
	}
}

// WithTimeout sets the overall request timeout on the underlying [http.Client].
func WithTimeout(d time.Duration) Option {
	return func(c *options) error {
		if d < 0 {
			return errors.New("timeout must not be negative")
		}
		c.timeout = &d
		return nil
	}
}

// WithUserAgent adds a persistent User-Agent header to all outgoing requests.
func WithUserAgent(header string) Option {
	return func(c *options) error {
		c.userAgent = header
		return nil
	}
}

// WithThrottle enables token-bucket rate limiting with the given requests per second and burst capacity.
func WithThrottle(rps, burst int) Option {
	return func(c *options) error {
		if rps <= 0 || burst <= 0 {
			return fmt.Errorf("rps[%d] and burst[%d] %w", rps, burst, throttle.ErrMustNotBeZero)
		}
		c.throttle = &throttle.Config{RPS: rps, Burst: burst}
		return nil
	}
}

// WithNoFollowRedirects prevents the [Client] from following HTTP redirects.
func WithNoFollowRedirects() Option {
	return func(c *options) error {
		c.noFollowRedirects = true
		return nil
	}
}

// WithLogger injects a custom [slog.Logger] into the [Client].
func WithLogger(logger *slog.Logger) Option {
	return func(c *options) error {
		c.logger = logger
		return nil
	}
}

// userAgent is an http.RoundTripper, enabling the persistent User-Agent header.
type userAgent struct {
	value string
	base  http.RoundTripper
}

func (ua userAgent) RoundTrip(r *http.Request) (*http.Response, error) {
	cpy := r.Clone(r.Context())
	cpy.Header.Set("User-Agent", ua.value)
	return ua.base.RoundTrip(cpy)
}

// DoOption is a functional option for [Client.Do].
type DoOption func(options *doOpts) error

type doOpts struct {
	responseBody any
	useJSONNum   bool
}

// WithDestination decodes the HTTP response body into bodyTemplate.
// bodyTemplate must be a pointer.
func WithDestination[T any](bodyTemplate *T) DoOption {
	return func(opts *doOpts) error {
		opts.responseBody = bodyTemplate

		return nil
	}
}

// WithJSONNumb tells the JSON decoder to use [json.Decoder.UseNumber],
// preserving number precision as [json.Number] instead of float64.
func WithJSONNumb() DoOption {
	return func(opts *doOpts) error {
		opts.useJSONNum = true

		return nil
	}
}

// RequestOption is a functional option for [Request].
type RequestOption func(options *requestOpts) error

type requestOpts struct {
	body        any
	contentType *string
	cookies     []*http.Cookie
	headers     map[string][]string
}

// WithPayload sets the JSON-encoded request body.
func WithPayload(body any) RequestOption {
	return func(opts *requestOpts) error {
		opts.body = body

		return nil
	}
}

// WithContentType overrides the default "application/json" Content-Type header.
func WithContentType(contentType string) RequestOption {
	return func(opts *requestOpts) error {
		if contentType == "" {
			return errors.New("cannot use empty content type")
		}

		opts.contentType = &contentType

		return nil
	}
}

// WithHeaders adds custom headers to the outgoing request.
func WithHeaders(headers map[string][]string) RequestOption {
	return func(opts *requestOpts) error {
		opts.headers = headers

		return nil
	}
}

// WithCookies attaches the given cookies to the outgoing request.
func WithCookies(cookies ...*http.Cookie) RequestOption {
	return func(opts *requestOpts) error {
		opts.cookies = cookies

		return nil
	}
}

// URLOption is a functional option for [URL].
type URLOption func(options *urlOpts)

type urlOpts struct {
	queryStrings map[string]string
	port         *int
}

// WithQueryStrings appends query parameters to the URL.
func WithQueryStrings(queryKV map[string]string) URLOption {
	return func(opts *urlOpts) {
		opts.queryStrings = queryKV
	}
}

// WithPort sets the port number on the URL's host.
func WithPort(port int) URLOption {
	return func(opts *urlOpts) {
		opts.port = &port
	}
}
