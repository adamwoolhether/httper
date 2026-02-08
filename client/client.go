// Package client exposes a series of helper functions for
// executing http requests against a remote server.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/adamwoolhether/httper/client/download"
	"github.com/adamwoolhether/httper/client/throttle"
)

// Client wraps the std-lib *http.Client
// It sets a default *http.Client and *http.Transport, which
// can be customized via optional funcs.
type Client struct {
	c      *http.Client
	logger *slog.Logger

	// dlQueue atomic.Value // *download.Queue
}

func Build(optFns ...Option) (*Client, error) {
	client := &Client{
		c:      http.DefaultClient,
		logger: slog.Default(),
	}

	var opts options
	for _, opt := range optFns {
		if err := opt(&opts); err != nil {
			return nil, fmt.Errorf("applying client option: %w", err)
		}
	}

	if opts.client != nil {
		client.c = opts.client
	}

	if opts.logger != nil {
		client.logger = opts.logger
	}

	if opts.timeout != nil {
		client.c.Timeout = *opts.timeout
	}

	if opts.noFollowRedirects {
		client.c.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	var transport http.RoundTripper
	switch {
	case opts.rt != nil:
		transport = opts.rt
	case opts.client != nil && opts.client.Transport != nil:
		transport = opts.client.Transport
	default:
		transport = http.DefaultTransport
	}
	if opts.userAgent != "" {
		transport = userAgent{value: opts.userAgent, base: transport}
	}
	if opts.throttle != nil {
		rt, err := throttle.NewRoundTripper(opts.throttle.RPS, opts.throttle.Burst, func() *slog.Logger { return client.logger }, transport)
		if err != nil {
			return nil, fmt.Errorf("configuring throttle: %w", err)
		}
		transport = rt
	}
	client.c.Transport = transport

	return client, nil
}

// Do will fire the request, and write response to the given dest object if any.
func (c *Client) Do(req *http.Request, expCode int, opts ...DoOption) error {
	var settings doOpts
	for _, opt := range opts {
		err := opt(&settings)
		if err != nil {
			return err
		}
	}

	doFunc := func(resp *http.Response) error {
		if settings.responseBody != nil {
			d := json.NewDecoder(resp.Body)

			if settings.useJSONNum {
				d.UseNumber()
			}

			if err := d.Decode(settings.responseBody); err != nil {
				return fmt.Errorf("decoding body: %w", err)
			}
		}

		return nil
	}

	return c.exec(req, expCode, doFunc)
}

// Download executes a request that's intended to stream the response body it to destPath.
// Data streams to a temp file in the same directory, then the temp file is renamed to
// destPath on success or cleared on failure
func (c *Client) Download(req *http.Request, expCode int, destPath string, opts ...DownloadOption) error {
	if destPath == "" {
		return errors.New("destPath must not be empty")
	}

	dlFunc := func(resp *http.Response) error {
		if err := download.Handle(req.Context(), resp.Body, resp.ContentLength, destPath, c.logger, opts...); err != nil {
			return fmt.Errorf("download: %w", err)
		}

		return nil
	}

	return c.exec(req, expCode, dlFunc)
}

// Request instantiates an *http.Request with the provided information.
// It's just a convenience method that wraps the public Request func.
func (c *Client) Request(ctx context.Context, reqURL *url.URL, method string, opts ...RequestOption) (*http.Request, error) {
	return Request(ctx, reqURL, method, opts...)
}

// URL creates a url.URL for use in Request.
// It's just a convenience method that wraps the public URL func.
func (c *Client) URL(scheme, host, path string, opts ...URLOption) *url.URL {
	return URL(scheme, host, path, opts...)
}

// exec runs the request and injected function on success after validating the expected status code.
func (c *Client) exec(req *http.Request, expCode int, fn execFn) error {
	resp, err := c.c.Do(req)
	if err != nil {
		return fmt.Errorf("exec http do: %w", err)
	}

	discardBody := true
	defer func() {
		if discardBody {
			if _, err = io.Copy(io.Discard, resp.Body); err != nil {
				c.logger.Error("failed to discard unused body", "error", err)
			}
		}
		if err = resp.Body.Close(); err != nil {
			c.logger.Error("failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != expCode {
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxErrBodySize))
		if err != nil {
			b = []byte("unable to read body")
		}

		return &UnexpectedStatusError{
			StatusCode: resp.StatusCode,
			Body:       string(b),
			Err:        ErrUnexpectedStatusCode,
		}
	}

	if err := fn(resp); err != nil {
		discardBody = false
		return fmt.Errorf("exec fn: %w", err)
	}

	return nil
}

// Request instantiates an *http.Request with the provided information.
// Content-Type defaults to `application/json` if unspecified via WithContentType.
func Request(ctx context.Context, reqURL *url.URL, method string, opts ...RequestOption) (*http.Request, error) {
	var settings requestOpts
	for _, opt := range opts {
		err := opt(&settings)
		if err != nil {
			return nil, err
		}
	}

	var payload bytes.Buffer
	if settings.body != nil {
		if err := json.NewEncoder(&payload).Encode(settings.body); err != nil {
			return nil, fmt.Errorf("encoding request payload: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), &payload)
	if err != nil {
		return nil, fmt.Errorf("instantiating request: %w", err)
	}

	for _, cookie := range settings.cookies {
		req.AddCookie(cookie)
	}

	var contentType string
	if settings.contentType == nil {
		contentType = "application/json"
	} else {
		contentType = *settings.contentType
	}

	req.Header.Set("Content-Type", contentType)
	for k, v := range settings.headers {
		for _, element := range v {
			req.Header.Add(k, element)
		}
	}

	return req, nil
}

// URL creates a url.URL for use in Request.
func URL(scheme, host, path string, opts ...URLOption) *url.URL {
	var settings urlOpts
	for _, opt := range opts {
		opt(&settings)
	}

	if settings.port != nil {
		host = fmt.Sprintf("%s:%d", host, *settings.port)
	}

	endpoint := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   path,
	}

	if settings.queryStrings != nil {
		queryParams := url.Values{}
		for k, v := range settings.queryStrings {
			queryParams.Add(k, v)
		}

		endpoint.RawQuery = queryParams.Encode()
	}

	return &endpoint
}
