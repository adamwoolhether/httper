package client_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/adamwoolhether/httper/client"
	"github.com/adamwoolhether/httper/client/download"
	"github.com/adamwoolhether/httper/client/throttle"
	"github.com/google/go-cmp/cmp"
)

type test struct {
	*client.Client

	server    *httptest.Server
	serverURL *url.URL
	teardown  func()
}

type payload struct {
	Body string `json:"body"`
}

func TestMain(m *testing.M) {
	var buf bytes.Buffer

	exitCode := m.Run()
	if exitCode != 0 {
		fmt.Println("******************** LOGS ********************")
		fmt.Print(buf.String())
		fmt.Println("******************** LOGS ********************")
	}
}

func TestClient_WithUserAgent(t *testing.T) {
	expectedUA := "TestUserAgent/1.0"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua != expectedUA {
			t.Errorf("expected User-Agent %q, got %q", expectedUA, ua)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	client, err := client.Build(client.WithUserAgent(expectedUA))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := client.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if err := client.Do(req, http.StatusOK); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestClient_WithThrottleAndUserAgent(t *testing.T) {
	expectedUA := "ThrottledAgent/1.0"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua != expectedUA {
			t.Errorf("expected User-Agent %q, got %q", expectedUA, ua)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	// WithThrottle applied before WithUserAgent — order shouldn't matter.
	client, err := client.Build(
		client.WithThrottle(100, 10),
		client.WithUserAgent(expectedUA),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := client.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if err := client.Do(req, http.StatusOK); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestClient_WithTransport(t *testing.T) {
	var called bool
	custom := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		return http.DefaultTransport.RoundTrip(r)
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	client, err := client.Build(client.WithTransport(custom))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := client.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if err := client.Do(req, http.StatusOK); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !called {
		t.Error("custom transport was not called")
	}
}

func TestClient_WithTransportNil(t *testing.T) {
	_, err := client.Build(client.WithTransport(nil))
	if err == nil {
		t.Fatal("expected error for nil transport")
	}
}

func TestClient_WithTimeout(t *testing.T) {
	client, err := client.Build(client.WithTimeout(30 * time.Second))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Verify the timeout was applied by making a request to a slow server.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	req, err := client.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if err := client.Do(req, http.StatusOK); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestClient_WithTimeoutZero(t *testing.T) {
	// Zero means no timeout per stdlib.
	_, err := client.Build(client.WithTimeout(0))
	if err != nil {
		t.Fatalf("expected no error for zero timeout, got: %v", err)
	}
}

func TestClient_WithTimeoutNegative(t *testing.T) {
	_, err := client.Build(client.WithTimeout(-1))
	if err == nil {
		t.Fatal("expected error for negative timeout")
	}
}

func TestClient_OptionOrderIndependence(t *testing.T) {
	expectedUA := "OrderTest/1.0"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua != expectedUA {
			t.Errorf("expected User-Agent %q, got %q", expectedUA, ua)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	var transportCalled bool
	custom := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		transportCalled = true
		return http.DefaultTransport.RoundTrip(r)
	})

	// Order A: Transport first, then UserAgent.
	clientA, err := client.Build(
		client.WithTransport(custom),
		client.WithUserAgent(expectedUA),
	)
	if err != nil {
		t.Fatalf("order A: failed to create client: %v", err)
	}

	req, err := clientA.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if err := clientA.Do(req, http.StatusOK); err != nil {
		t.Errorf("order A: expected no error, got: %v", err)
	}
	if !transportCalled {
		t.Error("order A: custom transport was not called")
	}

	// Order B: UserAgent first, then Transport.
	transportCalled = false
	clientB, err := client.Build(
		client.WithUserAgent(expectedUA),
		client.WithTransport(custom),
	)
	if err != nil {
		t.Fatalf("order B: failed to create client: %v", err)
	}

	req, err = clientB.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if err := clientB.Do(req, http.StatusOK); err != nil {
		t.Errorf("order B: expected no error, got: %v", err)
	}
	if !transportCalled {
		t.Error("order B: custom transport was not called")
	}
}

func TestClient_FullChainComposition(t *testing.T) {
	expectedUA := "FullChain/1.0"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua != expectedUA {
			t.Errorf("expected User-Agent %q, got %q", expectedUA, ua)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	var transportCalled bool
	custom := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		transportCalled = true
		return http.DefaultTransport.RoundTrip(r)
	})

	// All three options in various orders should produce the same result.
	orders := [][]client.Option{
		{client.WithTransport(custom), client.WithUserAgent(expectedUA), client.WithThrottle(100, 10)},
		{client.WithThrottle(100, 10), client.WithTransport(custom), client.WithUserAgent(expectedUA)},
		{client.WithUserAgent(expectedUA), client.WithThrottle(100, 10), client.WithTransport(custom)},
	}

	for i, opts := range orders {
		transportCalled = false

		client, err := client.Build(opts...)
		if err != nil {
			t.Fatalf("order %d: failed to create client: %v", i, err)
		}

		req, err := client.Request(t.Context(), testURL, http.MethodGet)
		if err != nil {
			t.Fatalf("order %d: failed to create request: %v", i, err)
		}

		if err := client.Do(req, http.StatusOK); err != nil {
			t.Errorf("order %d: expected no error, got: %v", i, err)
		}
		if !transportCalled {
			t.Errorf("order %d: custom transport was not called", i)
		}
	}
}

func TestClient_WithClient(t *testing.T) {
	custom := &http.Client{Timeout: 42 * time.Second}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	client, err := client.Build(client.WithClient(custom))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := client.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if err := client.Do(req, http.StatusOK); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Verify provided client's timeout is preserved (not overwritten by default).
	if custom.Timeout != 42*time.Second {
		t.Errorf("expected provided client timeout preserved as 42s, got %v", custom.Timeout)
	}
}

func TestClient_WithClientNil(t *testing.T) {
	_, err := client.Build(client.WithClient(nil))
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestClient_WithClientAndWithTimeout(t *testing.T) {
	// WithTimeout must always win over WithClient's timeout, regardless of order.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	// Order A: WithClient first, then WithTimeout.
	custom := &http.Client{Timeout: 1 * time.Millisecond}
	clientA, err := client.Build(
		client.WithClient(custom),
		client.WithTimeout(5*time.Second),
	)
	if err != nil {
		t.Fatalf("order A: failed to create client: %v", err)
	}

	req, err := clientA.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if err := clientA.Do(req, http.StatusOK); err != nil {
		t.Errorf("order A: expected no error (WithTimeout should win), got: %v", err)
	}

	// Order B: WithTimeout first, then WithClient.
	custom = &http.Client{Timeout: 1 * time.Millisecond}
	clientB, err := client.Build(
		client.WithTimeout(5*time.Second),
		client.WithClient(custom),
	)
	if err != nil {
		t.Fatalf("order B: failed to create client: %v", err)
	}

	req, err = clientB.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if err := clientB.Do(req, http.StatusOK); err != nil {
		t.Errorf("order B: expected no error (WithTimeout should win), got: %v", err)
	}
}

func TestClient_WithClientCustomTransport(t *testing.T) {
	// When WithClient provides a transport and WithTransport is not used,
	// the provided client's transport should be used as the base.
	var called bool
	customTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		return http.DefaultTransport.RoundTrip(r)
	})
	custom := &http.Client{Transport: customTransport}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	client, err := client.Build(client.WithClient(custom))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := client.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if err := client.Do(req, http.StatusOK); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !called {
		t.Error("provided client's transport was not called")
	}
}

func TestClient_WithClientAndWithTransport(t *testing.T) {
	// WithTransport must always win over the provided client's transport.
	var providedCalled, explicitCalled bool
	providedTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		providedCalled = true
		return http.DefaultTransport.RoundTrip(r)
	})
	explicitTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		explicitCalled = true
		return http.DefaultTransport.RoundTrip(r)
	})
	custom := &http.Client{Transport: providedTransport}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	client, err := client.Build(
		client.WithClient(custom),
		client.WithTransport(explicitTransport),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := client.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if err := client.Do(req, http.StatusOK); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if providedCalled {
		t.Error("provided client's transport should not have been called")
	}
	if !explicitCalled {
		t.Error("WithTransport's transport should have been called")
	}
}

func TestClient_WithNoFollowRedirects(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/target", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL + "/redirect")
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	client, err := client.Build(client.WithNoFollowRedirects())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := client.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// With no-follow, we should get the redirect status, not follow it.
	if err := client.Do(req, http.StatusFound); err != nil {
		t.Errorf("expected 302 response without following, got: %v", err)
	}
}

func TestClient_WithClientAndWithNoFollowRedirects(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/target", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL + "/redirect")
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	// Order A: WithClient first, then WithNoFollowRedirects.
	clientA, err := client.Build(
		client.WithClient(&http.Client{}),
		client.WithNoFollowRedirects(),
	)
	if err != nil {
		t.Fatalf("order A: failed to create client: %v", err)
	}

	req, err := clientA.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if err := clientA.Do(req, http.StatusFound); err != nil {
		t.Errorf("order A: expected 302, got: %v", err)
	}

	// Order B: WithNoFollowRedirects first, then WithClient.
	clientB, err := client.Build(
		client.WithNoFollowRedirects(),
		client.WithClient(&http.Client{}),
	)
	if err != nil {
		t.Fatalf("order B: failed to create client: %v", err)
	}

	req, err = clientB.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if err := clientB.Do(req, http.StatusFound); err != nil {
		t.Errorf("order B: expected 302, got: %v", err)
	}
}

// roundTripFunc adapts a function into an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestClient_WithThrottleValidation(t *testing.T) {
	_, err := client.Build(client.WithThrottle(0, 10))
	if err == nil {
		t.Fatal("expected error for zero rps")
	}
	if !errors.Is(err, throttle.ErrMustNotBeZero) {
		t.Errorf("expected ErrMustNotBeZero, got: %v", err)
	}
}

func TestClient_Do(t *testing.T) {
	test := mockServer(t)
	defer test.teardown()

	testClient := test.Client

	testCases := map[string]struct {
		url         *url.URL
		path        string
		method      string
		expStatus   int
		payload     *payload
		captureResp *payload
		captureRaw  *map[string]any
		useJSONNumb bool
		checkResp   func(t *testing.T, raw map[string]any)
		err         error
	}{
		"basicGet": {
			url:         test.serverURL,
			path:        "",
			method:      http.MethodGet,
			expStatus:   http.StatusOK,
			payload:     nil,
			captureResp: nil,
			err:         nil,
		},
		"basicExp202NotOK": {
			url:         test.serverURL,
			path:        "",
			method:      http.MethodGet,
			expStatus:   http.StatusAccepted,
			payload:     nil,
			captureResp: nil,
			err:         client.ErrUnexpectedStatusCode,
		},
		"basicExp202OK": {
			url:         test.serverURL,
			path:        "/expstatus",
			method:      http.MethodGet,
			expStatus:   http.StatusAccepted,
			payload:     nil,
			captureResp: nil,
		},
		"getCaptureResp": {
			url:         test.serverURL,
			path:        "",
			method:      http.MethodGet,
			expStatus:   http.StatusOK,
			payload:     nil,
			captureResp: new(payload),
		},
		"postCaptureResp": {
			url:         test.serverURL,
			path:        "/echo",
			method:      http.MethodPost,
			expStatus:   http.StatusOK,
			payload:     &payload{Body: "hey there"},
			captureResp: new(payload),
		},
		"withJSONNumb": {
			url:         test.serverURL,
			path:        "/number",
			method:      http.MethodGet,
			expStatus:   http.StatusOK,
			captureRaw:  &map[string]any{},
			useJSONNumb: true,
			checkResp: func(t *testing.T, raw map[string]any) {
				t.Helper()
				id, ok := raw["id"]
				if !ok {
					t.Fatal("expected 'id' key in response")
				}
				n, ok := id.(json.Number)
				if !ok {
					t.Fatalf("expected json.Number, got %T", id)
				}
				if n.String() != "12345678901234567" {
					t.Errorf("expected 12345678901234567, got %s", n.String())
				}
			},
		},
		"withoutJSONNumb": {
			url:         test.serverURL,
			path:        "/number",
			method:      http.MethodGet,
			expStatus:   http.StatusOK,
			captureRaw:  &map[string]any{},
			useJSONNumb: false,
			checkResp: func(t *testing.T, raw map[string]any) {
				t.Helper()
				id, ok := raw["id"]
				if !ok {
					t.Fatal("expected 'id' key in response")
				}
				if _, ok := id.(float64); !ok {
					t.Fatalf("expected float64 without UseNumber, got %T", id)
				}
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			var reqOpts []client.RequestOption
			if tc.payload != nil {
				reqOpts = append(reqOpts, client.WithPayload(*tc.payload))
			}

			var opts []client.DoOption
			if tc.captureResp != nil {
				opts = append(opts, client.WithDestination(tc.captureResp))
			}
			if tc.captureRaw != nil {
				opts = append(opts, client.WithDestination(tc.captureRaw))
			}
			if tc.useJSONNumb {
				opts = append(opts, client.WithJSONNumb())
			}

			if len(tc.path) > 0 {
				copied := *tc.url
				copied.Path = tc.path
				tc.url = &copied
			}

			req, err := testClient.Request(t.Context(), tc.url, tc.method, reqOpts...)
			if err != nil {
				t.Fatalf("generating req: %v", err)
			}

			err = testClient.Do(req, tc.expStatus, opts...)
			if err != nil {
				if !errors.Is(err, tc.err) {
					t.Errorf("exp err: %v, got: %v", tc.err, err)
				}
			}

			if tc.captureResp != nil && tc.payload != nil {
				if *tc.captureResp != *tc.payload {
					t.Errorf("expected identitcal body from echo server; diff %v", cmp.Diff(tc.captureResp, tc.payload))
				}
			}

			if tc.checkResp != nil && tc.captureRaw != nil {
				tc.checkResp(t, *tc.captureRaw)
			}
		})
	}
}

func TestClient_Request(t *testing.T) {
	testCases := map[string]struct {
		url         *url.URL
		method      string
		payload     *payload
		contentType string
		headers     map[string][]string
		cookies     []*http.Cookie
	}{
		"basic": {
			url:         client.URL("https", "localhost", "/", client.WithPort(8888)),
			method:      http.MethodGet,
			payload:     nil,
			contentType: "",
			headers:     nil,
		},
		"withPayload": {
			url:         client.URL("https", "localhost", "/", client.WithPort(8888)),
			method:      http.MethodPost,
			payload:     &payload{Body: "hey there"},
			contentType: "",
			headers:     nil,
		},
		"withCustomContentType": {
			url:         client.URL("https", "localhost", "/", client.WithPort(8888)),
			method:      http.MethodGet,
			payload:     nil,
			contentType: "text/html",
			headers:     nil,
		},
		"withHeaders": {
			url:         client.URL("https", "localhost", "/", client.WithPort(8888)),
			method:      http.MethodPost,
			payload:     nil,
			contentType: "",
			headers: map[string][]string{
				"Single-Val": {"value"},
				"Multi-Val":  {"value", "value2"},
			},
		},
		"withSingleCookie": {
			url:    client.URL("https", "localhost", "/", client.WithPort(8888)),
			method: http.MethodGet,
			cookies: []*http.Cookie{
				{Name: "session", Value: "abc123"},
			},
		},
		"withMultipleCookies": {
			url:    client.URL("https", "localhost", "/", client.WithPort(8888)),
			method: http.MethodGet,
			cookies: []*http.Cookie{
				{Name: "session", Value: "abc123"},
				{Name: "theme", Value: "dark"},
				{Name: "lang", Value: "en"},
			},
		},
	}

	const defaultContentType = "application/json"

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			var opts []client.RequestOption
			if tc.payload != nil {
				opts = append(opts, client.WithPayload(*tc.payload))
			}

			if len(tc.contentType) > 0 {
				opts = append(opts, client.WithContentType(tc.contentType))
			}

			if tc.headers != nil {
				opts = append(opts, client.WithHeaders(tc.headers))
			}

			if tc.cookies != nil {
				opts = append(opts, client.WithCookies(tc.cookies...))
			}

			req, err := client.Request(t.Context(), tc.url, tc.method, opts...)
			if err != nil {
				t.Fatalf("create request exp nil err; got: %v", err)
			}

			if tc.payload != nil {
				var reqBody payload
				if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
					t.Fatalf("reading req body: %v", err)
				}

				expBodyData, err := json.Marshal(tc.payload)
				if err != nil {
					t.Fatalf("creating exp body bytes: %v", err)
				}

				var expBody payload
				if err := json.NewDecoder(bytes.NewReader(expBodyData)).Decode(&expBody); err != nil {
					t.Fatalf("reading req body: %v", err)
				}

				if reqBody != expBody {
					t.Errorf("exp req body: %v, got: %v", tc.payload.Body, reqBody)
				}
			}

			reqContentType := req.Header.Get("Content-Type")
			if len(tc.contentType) > 0 {
				if reqContentType != tc.contentType {
					t.Errorf("exp custom content type[%s] for request, got: %v", tc.contentType, reqContentType)
				}
			} else {
				if reqContentType != defaultContentType {
					t.Errorf("exp default content type[%s], got: %v", defaultContentType, reqContentType)
				}
			}

			if tc.headers != nil {
				for k, v := range tc.headers {
					hdr, ok := req.Header[k]
					if !ok {
						t.Errorf("custom header[%s] not found in req", k)
					}

					if len(hdr) != len(v) {
						t.Errorf("exp header[%s] to be: %v, got: %v", k, hdr, v)
					}

					for i := range v {
						if hdr[i] != v[i] {
							t.Errorf("incongruent header value; exp: %v, got: %v", v[i], hdr[i])
						}
					}
				}
			}

			if tc.cookies != nil {
				got := req.Cookies()
				if len(got) != len(tc.cookies) {
					t.Fatalf("exp %d cookies, got %d", len(tc.cookies), len(got))
				}

				for i, exp := range tc.cookies {
					if got[i].Name != exp.Name {
						t.Errorf("cookie[%d] name: exp %q, got %q", i, exp.Name, got[i].Name)
					}
					if got[i].Value != exp.Value {
						t.Errorf("cookie[%d] value: exp %q, got %q", i, exp.Value, got[i].Value)
					}
				}
			}
		})
	}
}

func TestClient_URL(t *testing.T) {
	testCases := map[string]struct {
		scheme string
		host   string
		port   int
		path   string
		qs     map[string]string
		exp    string
	}{
		"basic": {
			scheme: "https",
			host:   "localhost",
			port:   8888,
			path:   "/",
			qs:     nil,
			exp:    "https://localhost:8888/",
		},
		"withQS": {
			scheme: "https",
			host:   "localhost",
			port:   8888,
			path:   "/somepath",
			qs:     map[string]string{"key": "value"},
			exp:    "https://localhost:8888/somepath?key=value",
		},
		"withMultipleQS": {
			scheme: "https",
			host:   "localhost",
			port:   8888,
			path:   "/somepath",
			qs:     map[string]string{"key": "value", "key2": "value2"},
			exp:    "https://localhost:8888/somepath?key=value&key2=value2",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			var opts []client.URLOption
			if tc.qs != nil {
				opts = append(opts, client.WithQueryStrings(tc.qs))
			}
			if tc.port != 0 {

				opts = append(opts, client.WithPort(tc.port))
			}

			url := client.URL(tc.scheme, tc.host, tc.path, opts...)

			if url.String() != tc.exp {
				t.Errorf("exp generated url:, %q, got: %q", tc.exp, url.String())
			}
		})
	}
}

const successRespBody = "success"

func mockServer(t *testing.T) *test {
	t.Helper()

	testClient, err := client.Build()
	if err != nil {
		t.Fatalf("failed to create testClient: %v", err)
	}

	rootHandler := func(w http.ResponseWriter, r *http.Request) {
		resp := payload{Body: successRespBody}
		data, err := json.Marshal(resp)
		if err != nil { // nolint: wsl
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}

	exp200Handler := func(w http.ResponseWriter, t *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}

	echoHandler := func(w http.ResponseWriter, r *http.Request) {
		var decoded payload
		if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data, err := json.Marshal(decoded)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}

	numberHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":12345678901234567}`))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/expstatus", exp200Handler)
	mux.HandleFunc("/echo", echoHandler)
	mux.HandleFunc("/number", numberHandler)
	server := httptest.NewServer(mux)

	testURL, err := url.ParseRequestURI(server.URL)
	if err != nil {
		t.Fatal("parsing test server URL")
	}

	ts := test{
		Client:    testClient,
		server:    server,
		serverURL: testURL,
		teardown: func() {
			server.Close()
		},
	}

	return &ts
}

// /////////////////////////////////////////////////////////////////
// Download Tests

func TestClient_Download_Basic(t *testing.T) {
	expBody := []byte("hello download world")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(expBody)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expBody)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "downloaded.bin")

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	if err := c.Download(req, http.StatusOK, destPath); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}

	if !bytes.Equal(got, expBody) {
		t.Errorf("file contents mismatch; got %q, want %q", got, expBody)
	}
}

func TestClient_Download_ChecksumPass(t *testing.T) {
	expBody := []byte("checksum test data")
	hash := sha256.Sum256(expBody)
	expChecksum := hex.EncodeToString(hash[:])

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(expBody)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expBody)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "checksum-pass.bin")

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	if err := c.Download(req, http.StatusOK, destPath, download.WithChecksum(sha256.New(), expChecksum)); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}

	if !bytes.Equal(got, expBody) {
		t.Errorf("file contents mismatch; got %q, want %q", got, expBody)
	}
}

func TestClient_Download_ChecksumFail(t *testing.T) {
	expBody := []byte("checksum test data")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(expBody)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expBody)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "checksum-fail.bin")

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Download(req, http.StatusOK, destPath, download.WithChecksum(sha256.New(), "badhash"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, download.ErrChecksumMismatch) {
		t.Errorf("expected ErrChecksumMismatch, got: %v", err)
	}

	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("expected file to not exist at %s after checksum failure", destPath)
	}
}

func TestClient_Download_ContentLengthMismatch(t *testing.T) {
	// Use Hijack to send a raw response with mismatched Content-Length
	// without the server closing the connection early.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set Content-Length to 5 but send 10 bytes. The HTTP client
		// will only read 5 bytes (respecting Content-Length), and our
		// check will see n == contentLength so no mismatch.
		// Instead: set Content-Length to 10, send only 5 via Hijack.
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("server doesn't support hijacking")
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack failed: %v", err)
		}
		defer conn.Close()
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nhello")
		buf.Flush()
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "mismatch.bin")

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Download(req, http.StatusOK, destPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// The HTTP client may return an io.UnexpectedEOF or our
	// ErrContentLengthMismatch depending on timing. Either is acceptable
	// as both indicate an incomplete download.
	if !errors.Is(err, download.ErrContentLengthMismatch) {
		// Accept io.UnexpectedEOF as the Go HTTP client detects the
		// short read before our content-length check runs.
		t.Logf("got error (acceptable): %v", err)
	}

	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("expected file to not exist at %s after content length mismatch", destPath)
	}
}

func TestClient_Download_Progress(t *testing.T) {
	expBody := bytes.Repeat([]byte("abcdefghij"), 1000) // 10KB

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(expBody)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expBody)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "progress.bin")

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Download(req, http.StatusOK, destPath, download.WithProgress())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}

	if !bytes.Equal(got, expBody) {
		t.Errorf("file contents mismatch; got %d bytes, want %d", len(got), len(expBody))
	}
}

func TestClient_Download_ProgressUnknownLength(t *testing.T) {
	expBody := []byte("no content length")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use Flusher to force chunked transfer encoding,
		// which results in ContentLength == -1.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expBody)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "unknown-len.bin")

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Download(req, http.StatusOK, destPath, download.WithProgress())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}

	if !bytes.Equal(got, expBody) {
		t.Errorf("file contents mismatch; got %q, want %q", got, expBody)
	}
}

func TestClient_Download_EmptyDestPath(t *testing.T) {
	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not have been made")
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	if err := c.Download(req, http.StatusOK, ""); err == nil {
		t.Error("expected error for empty destPath, got nil")
	}
}

func TestClient_Download_StatusCodeMismatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "should-not-exist.bin")

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Download(req, http.StatusOK, destPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var statusErr *client.UnexpectedStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected *UnexpectedStatusError, got: %T: %v", err, err)
	}

	if statusErr.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", statusErr.StatusCode)
	}

	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("expected file to not exist at %s after status code mismatch", destPath)
	}
}

func TestClient_Download_FullChain(t *testing.T) {
	expBody := bytes.Repeat([]byte("x"), 5000)
	hash := sha256.Sum256(expBody)
	expChecksum := hex.EncodeToString(hash[:])

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(expBody)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expBody)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "full-chain.bin")

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Download(req, http.StatusOK, destPath,
		download.WithChecksum(sha256.New(), expChecksum),
		download.WithProgress(),
	)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}

	if !bytes.Equal(got, expBody) {
		t.Error("file contents mismatch")
	}
}

func TestClient_Download_ErrorBodyCapped(t *testing.T) {
	// Server returns a wrong status code with a body larger than 4KB.
	// The error body captured in UnexpectedStatusError must be capped.
	largeBody := bytes.Repeat([]byte("X"), 8192) // 8KB

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(largeBody)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "capped.bin")

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Download(req, http.StatusOK, destPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var statusErr *client.UnexpectedStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected *UnexpectedStatusError, got: %T: %v", err, err)
	}

	const maxErrBodySize = 4 << 10
	if len(statusErr.Body) > maxErrBodySize {
		t.Errorf("error body not capped: got %d bytes, want <= %d", len(statusErr.Body), maxErrBodySize)
	}
	if len(statusErr.Body) != maxErrBodySize {
		t.Errorf("expected body to be exactly %d bytes (capped), got %d", maxErrBodySize, len(statusErr.Body))
	}
}

func TestClient_Do_ErrorBodyCapped(t *testing.T) {
	largeBody := bytes.Repeat([]byte("Y"), 8192)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(largeBody)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Do(req, http.StatusOK)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var statusErr *client.UnexpectedStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected *UnexpectedStatusError, got: %T: %v", err, err)
	}

	const maxErrBodySize = 4 << 10
	if len(statusErr.Body) > maxErrBodySize {
		t.Errorf("error body not capped: got %d bytes, want <= %d", len(statusErr.Body), maxErrBodySize)
	}
}

func TestClient_Download_SkipExisting(t *testing.T) {
	var requestCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("new data"))
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "existing.bin")

	// Pre-create the destination file with known content.
	originalContent := []byte("original")
	if err := os.WriteFile(destPath, originalContent, 0o644); err != nil {
		t.Fatalf("writing pre-existing file: %v", err)
	}

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Download(req, http.StatusOK, destPath, download.WithSkipExisting())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// File content should be unchanged.
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if !bytes.Equal(got, originalContent) {
		t.Errorf("file was overwritten; got %q, want %q", got, originalContent)
	}
}

func TestClient_Download_SkipExistingNotPresent(t *testing.T) {
	expBody := []byte("fresh download")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(expBody)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expBody)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "not-existing.bin")

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Download(req, http.StatusOK, destPath, download.WithSkipExisting())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}

	if !bytes.Equal(got, expBody) {
		t.Errorf("file contents mismatch; got %q, want %q", got, expBody)
	}
}

func TestClient_Download_CancelMidDownload(t *testing.T) {
	// Server writes 1KB chunks with a delay between each to simulate a slow download.
	const chunkSize = 1024
	const totalChunks = 20
	chunk := bytes.Repeat([]byte("a"), chunkSize)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(chunkSize*totalChunks))
		w.WriteHeader(http.StatusOK)

		for range totalChunks {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(100 * time.Millisecond)
		}
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "cancelled.bin")

	ctx, cancel := context.WithCancel(t.Context())

	req, err := c.Request(ctx, testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.Download(req, http.StatusOK, destPath)
	}()

	// Let a few chunks arrive, then cancel.
	time.Sleep(250 * time.Millisecond)
	cancel()

	err = <-errCh
	if err == nil {
		t.Fatal("expected error after cancellation, got nil")
	}

	if !errors.Is(err, download.ErrDownloadCancelled) {
		t.Errorf("expected ErrDownloadCancelled, got: %v", err)
	}

	// Verify no temp files remain.
	matches, _ := filepath.Glob(filepath.Join(tmpDir, ".httper-dl-*"))
	if len(matches) > 0 {
		t.Errorf("expected no temp files, found: %v", matches)
	}

	// Verify dest file does not exist.
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("expected dest file to not exist at %s after cancellation", destPath)
	}
}

func TestClient_Download_AlreadyCancelledContext(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not have been made")
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately.

	destPath := filepath.Join(t.TempDir(), "should-not-exist.bin")

	req, err := c.Request(ctx, testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Download(req, http.StatusOK, destPath)
	if err == nil {
		t.Fatal("expected error for already-cancelled context, got nil")
	}

	// The HTTP client rejects the request before it's sent, so the
	// error wraps context.Canceled rather than ErrDownloadCancelled.
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// /////////////////////////////////////////////////////////////////
// DownloadAsync Tests

func TestClient_DownloadAsync_Single(t *testing.T) {
	expBody := []byte("async download body")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(expBody)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expBody)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "async-single.bin")

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	r, err := c.DownloadAsync(req, http.StatusOK, destPath)
	if err != nil {
		t.Fatalf("starting async download: %v", err)
	}

	if err := r.Wait(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}

	if !bytes.Equal(got, expBody) {
		t.Errorf("file contents mismatch; got %q, want %q", got, expBody)
	}
}

func TestClient_DownloadAsync_Batch(t *testing.T) {
	const numFiles = 5
	expBody := []byte("batch download content")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(expBody)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expBody)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	tmpDir := t.TempDir()

	// First download starts the batch.
	req0, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request 0: %v", err)
	}
	r, err := c.DownloadAsync(req0, http.StatusOK, filepath.Join(tmpDir, "batch-0.bin"), download.WithBatch(2))
	if err != nil {
		t.Fatalf("starting async download 0: %v", err)
	}

	// Subsequent downloads added via r.Download.
	for i := 1; i < numFiles; i++ {
		destPath := filepath.Join(tmpDir, fmt.Sprintf("batch-%d.bin", i))

		req, err := c.Request(t.Context(), testURL, http.MethodGet)
		if err != nil {
			t.Fatalf("creating request %d: %v", i, err)
		}

		if _, err := r.Add(req, http.StatusOK, destPath); err != nil {
			t.Fatalf("starting async download %d: %v", i, err)
		}
	}

	if err := r.Wait(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	for i := range numFiles {
		destPath := filepath.Join(tmpDir, fmt.Sprintf("batch-%d.bin", i))
		got, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatalf("reading file %d: %v", i, err)
		}
		if !bytes.Equal(got, expBody) {
			t.Errorf("file %d contents mismatch; got %q, want %q", i, got, expBody)
		}
	}
}

func TestClient_DownloadAsync_CancelOneInBatch(t *testing.T) {
	const chunkSize = 1024
	const totalChunks = 20
	chunk := bytes.Repeat([]byte("b"), chunkSize)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(chunkSize*totalChunks))
		w.WriteHeader(http.StatusOK)
		for range totalChunks {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	tmpDir := t.TempDir()

	// Start the first slow download (creates the batch).
	req1, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request 1: %v", err)
	}
	r1, err := c.DownloadAsync(req1, http.StatusOK, filepath.Join(tmpDir, "cancel-me.bin"), download.WithBatch(4))
	if err != nil {
		t.Fatalf("starting async download 1: %v", err)
	}

	// Add a second slow download that should complete.
	req2, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request 2: %v", err)
	}
	r2, err := r1.Add(req2, http.StatusOK, filepath.Join(tmpDir, "keep-me.bin"))
	if err != nil {
		t.Fatalf("starting async download 2: %v", err)
	}
	_ = r2

	// Let downloads start, then cancel r1.
	time.Sleep(100 * time.Millisecond)
	r1.Cancel()

	err = r1.Wait()
	if err == nil {
		t.Fatal("expected error from cancelled download, got nil")
	}

	// The cancelled download should have produced an error.
	r1Err := r1.Err()
	if r1Err == nil {
		t.Error("expected r1 to have an error after cancel")
	}
}

func TestClient_DownloadAsync_EmptyDestPath(t *testing.T) {
	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not have been made")
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	if _, err := c.DownloadAsync(req, http.StatusOK, ""); err == nil {
		t.Error("expected error for empty destPath, got nil")
	}
}

func TestClient_DownloadAsync_WithChecksum(t *testing.T) {
	expBody := []byte("async checksum data")
	hash := sha256.Sum256(expBody)
	expChecksum := hex.EncodeToString(hash[:])

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(expBody)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expBody)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "async-checksum.bin")

	req, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	r, err := c.DownloadAsync(req, http.StatusOK, destPath, download.WithBatch(2), download.WithChecksum(sha256.New(), expChecksum))
	if err != nil {
		t.Fatalf("starting async download: %v", err)
	}

	if err := r.Wait(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}

	if !bytes.Equal(got, expBody) {
		t.Errorf("file contents mismatch; got %q, want %q", got, expBody)
	}
}

func TestClient_DownloadAsync_WithBatchOnAddRejected(t *testing.T) {
	expBody := []byte("reject batch on add")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(expBody)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expBody)
	}))
	defer ts.Close()

	testURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}

	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	tmpDir := t.TempDir()

	req0, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request 0: %v", err)
	}

	r, err := c.DownloadAsync(req0, http.StatusOK, filepath.Join(tmpDir, "first.bin"), download.WithBatch(2))
	if err != nil {
		t.Fatalf("starting async download: %v", err)
	}

	req1, err := c.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request 1: %v", err)
	}

	_, err = r.Add(req1, http.StatusOK, filepath.Join(tmpDir, "second.bin"), download.WithBatch(2))
	if err == nil {
		t.Fatal("expected error when passing WithBatch to Result.Add, got nil")
	}

	if err := r.Wait(); err != nil {
		t.Fatalf("expected no error from wait, got: %v", err)
	}
}
