// Package httper exposes a series of helper functions for
// executing http requests against a remote server.
package httper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Client enables use of a http client for a given device.
// It sets a default *http.Client and *http.Transport, which
// can be customized via optional funcs.
type Client struct {
	c      *http.Client
	logger *slog.Logger
}

func New(options ...ClientOption) (*Client, error) {
	baseTransport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
		// TLSClientConfig: tlsConf,
		MaxIdleConns: 5,
	}

	client := &Client{
		c: &http.Client{
			Transport: baseTransport,
			Timeout:   10 * time.Second,
		},
		logger: slog.Default(),
	}

	for _, opt := range options {
		if err := opt(client); err != nil {
			return nil, fmt.Errorf("applying client option: %w", err)
		}
	}

	return client, nil
}

// Do will fire the request, and write response to the given dest object if any.
func (dc *Client) Do(req *http.Request, expCode int, opts ...DoOption) error {
	var settings doOpts
	for _, opt := range opts {
		err := opt(&settings)
		if err != nil {
			return err
		}
	}

	resp, err := dc.c.Do(req)
	if err != nil {
		return fmt.Errorf("exec http do: %w", err)
	}

	var shouldExhaust bool
	defer func() {
		if settings.responseBody == nil || shouldExhaust {
			if _, err = io.Copy(io.Discard, resp.Body); err != nil {
				if dc.logger != nil {
					dc.logger.Error("failed to discard unused body", "error", err)
				}
			}
		}

		if err = resp.Body.Close(); err != nil {
			if dc.logger != nil {
				dc.logger.Error("failed to close response body", "error", err)
			}
		}
	}()

	if resp.StatusCode != expCode {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			shouldExhaust = true
			return &UnexpectedStatusError{
				StatusCode: resp.StatusCode,
				Body:       "unable to read body",
				Err:        ErrUnexpectedStatusCode,
			}
		}

		if resp.StatusCode == http.StatusUnauthorized {
			return &UnexpectedStatusError{
				StatusCode: resp.StatusCode,
				Body:       string(b),
				Err:        fmt.Errorf("%w: %w", ErrAuthenticationFailed, ErrUnexpectedStatusCode),
			}
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
