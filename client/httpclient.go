// Package httper exposes a series of helper functions for
// executing http requests against a remote server.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/adamwoolhether/httper/client/throttle"
)

// Client wraps the std-lib *http.Client
// It sets a default *http.Client and *http.Transport, which
// can be customized via optional funcs.
type Client struct {
	c      *http.Client
	logger *slog.Logger
}

func Build(opts ...ClientOption) (*Client, error) {
	client := &Client{
		c:      http.DefaultClient,
		logger: slog.Default(),
	}

	var options clientOpts
	for _, opt := range opts {
		if err := opt(&options); err != nil {
			return nil, fmt.Errorf("applying client option: %w", err)
		}
	}

	if options.client != nil {
		client.c = options.client
	}

	if options.logger != nil {
		client.logger = options.logger
	}

	if options.timeout != nil {
		client.c.Timeout = *options.timeout
	}
	if options.noFollowRedirects {
		client.c.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	var transport http.RoundTripper
	switch {
	case options.rt != nil:
		transport = options.rt
	case options.client != nil && options.client.Transport != nil:
		transport = options.client.Transport
	default:
		transport = http.DefaultTransport
	}
	if options.userAgent != "" {
		transport = userAgent{value: options.userAgent, base: transport}
	}
	if options.throttle != nil {
		rt, err := throttle.NewRoundTripper(options.throttle.RPS, options.throttle.Burst, func() *slog.Logger { return client.logger }, transport)
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

	resp, err := c.c.Do(req)
	if err != nil {
		return fmt.Errorf("exec http do: %w", err)
	}

	var shouldExhaust bool
	defer func() {
		if settings.responseBody == nil || shouldExhaust {
			if _, err = io.Copy(io.Discard, resp.Body); err != nil {
				c.logger.Error("failed to discard unused body", "error", err)
			}
		}

		if err = resp.Body.Close(); err != nil {
			c.logger.Error("failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != expCode {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			shouldExhaust = true
			b = []byte("unable to read body")
		}

		return &UnexpectedStatusError{
			StatusCode: resp.StatusCode,
			Body:       string(b),
			Err:        ErrUnexpectedStatusCode,
		}
	}

	if settings.responseBody != nil {
		d := json.NewDecoder(resp.Body)

		if settings.useJSONNum {
			d.UseNumber()
		}

		if err := d.Decode(settings.responseBody); err != nil {
			return fmt.Errorf("failed to decode body: %w", err)
		}
	}

	return nil
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
