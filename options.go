package httper

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/adamwoolhether/httper/throttle"
)

// ClientOption defines optional settings for the http client.
//
// WithUserAgent adds a persistent `User-Agent` header to all
// outgoing requests on the client.
type ClientOption func(*Client) error

func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) error {
		c.logger = logger
		return nil
	}
}

func WithUserAgent(header string) ClientOption {
	return func(c *Client) error {
		base := c.c.Transport
		if base == nil {
			base = http.DefaultTransport
		}

		c.c.Transport = userAgent{value: header, base: base}
		return nil
	}
}

func WithThrottle(rps, burst int) ClientOption {
	return func(c *Client) error {
		base := c.c.Transport
		if base == nil {
			base = http.DefaultTransport
		}

		rt, err := throttle.NewRoundTripper(rps, burst, func() *slog.Logger { return c.logger }, base)
		if err != nil {
			return fmt.Errorf("configuring throttle: %w", err)
		}

		c.c.Transport = rt
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
