package client

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/adamwoolhether/httper/client/download"
	"github.com/adamwoolhether/httper/client/throttle"
)

// Option defines optional settings for the http client.
//
// WithLogger injects a custom logger into the client.
// WithUserAgent adds a persistent `User-Agent` header to all
// outgoing requests on the client.
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

func WithClient(hc *http.Client) Option {
	return func(c *options) error {
		if hc == nil {
			return errors.New("client must not be nil")
		}
		c.client = hc
		return nil
	}
}

func WithTransport(rt http.RoundTripper) Option {
	return func(c *options) error {
		if rt == nil {
			return errors.New("transport must not be nil")
		}
		c.rt = rt
		return nil
	}
}

func WithTimeout(d time.Duration) Option {
	return func(c *options) error {
		if d < 0 {
			return errors.New("timeout must not be negative")
		}
		c.timeout = &d
		return nil
	}
}

func WithUserAgent(header string) Option {
	return func(c *options) error {
		c.userAgent = header
		return nil
	}
}

func WithThrottle(rps, burst int) Option {
	return func(c *options) error {
		if rps <= 0 || burst <= 0 {
			return fmt.Errorf("rps[%d] and burst[%d] %w", rps, burst, throttle.ErrMustNotBeZero)
		}
		c.throttle = &throttle.Config{RPS: rps, Burst: burst}
		return nil
	}
}

func WithNoFollowRedirects() Option {
	return func(c *options) error {
		c.noFollowRedirects = true
		return nil
	}
}

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

// /////////////////////////////////////////////////////////////////

// DoOption defines optional settings for *Client.Do.
//
// WithDestination enables capturing http response body with the
// given struct template. bodyTemplate struct MUST be a pointer.
// WithJSONNumb tells the decoder to use decoder.UseNumber().
type DoOption func(options *doOpts) error

type doOpts struct {
	responseBody any
	useJSONNum   bool
}

func WithDestination[T any](bodyTemplate *T) DoOption {
	return func(opts *doOpts) error {
		opts.responseBody = bodyTemplate

		return nil
	}
}

func WithJSONNumb() DoOption {
	return func(opts *doOpts) error {
		opts.useJSONNum = true

		return nil
	}
}

// /////////////////////////////////////////////////////////////////

// RequestOption defines optional settings for *Client.Request
//
// WithPayload enables setting a body for the outgoing Request.
// WithContentType enables setting the Content-Type header.
// WithHeaders enables setting custom headers.
// WithCookies enables injecting cookie(s) into the request.
type RequestOption func(options *requestOpts) error

type requestOpts struct {
	body        any
	contentType *string
	cookies     []*http.Cookie
	headers     map[string][]string
}

func WithPayload(body any) RequestOption {
	return func(opts *requestOpts) error {
		opts.body = body

		return nil
	}
}

func WithContentType(contentType string) RequestOption {
	return func(opts *requestOpts) error {
		if contentType == "" {
			return errors.New("cannot use empty content type")
		}

		opts.contentType = &contentType

		return nil
	}
}

func WithHeaders(headers map[string][]string) RequestOption {
	return func(opts *requestOpts) error {
		opts.headers = headers

		return nil
	}
}

func WithCookies(cookies ...*http.Cookie) RequestOption {
	return func(opts *requestOpts) error {
		opts.cookies = cookies

		return nil
	}
}

// /////////////////////////////////////////////////////////////////

// URLOption enables settings for constructing a url.URL.
// WithQueryStrings enables providing query strings to url.URL.
// WithPort enables adding a port number to the host field.
type URLOption func(options *urlOpts)

type urlOpts struct {
	queryStrings map[string]string
	port         *int
}

func WithQueryStrings(queryKV map[string]string) URLOption {
	return func(opts *urlOpts) {
		opts.queryStrings = queryKV
	}
}

func WithPort(port int) URLOption {
	return func(opts *urlOpts) {
		opts.port = &port
	}
}

// DownloadOption defines optional settings for downloading files.
type DownloadOption = download.Option
