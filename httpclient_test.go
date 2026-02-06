package httper_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/adamwoolhether/httper"
	"github.com/adamwoolhether/httper/throttle"
	"github.com/google/go-cmp/cmp"
)

type test struct {
	*httper.Client

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

	client, err := httper.New(httper.WithUserAgent(expectedUA))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := httper.Request(t.Context(), testURL, http.MethodGet)
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
	client, err := httper.New(
		httper.WithThrottle(100, 10),
		httper.WithUserAgent(expectedUA),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := httper.Request(t.Context(), testURL, http.MethodGet)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if err := client.Do(req, http.StatusOK); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestClient_WithThrottleValidation(t *testing.T) {
	_, err := httper.New(httper.WithThrottle(0, 10))
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

	client := test.Client

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
			err:         httper.ErrUnexpectedStatusCode,
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

	const dlFileName = "test.json"

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			var reqOpts []httper.RequestOption
			if tc.payload != nil {
				reqOpts = append(reqOpts, httper.WithPayload(*tc.payload))
			}

			var opts []httper.DoOption
			if tc.captureResp != nil {
				opts = append(opts, httper.WithDestination(tc.captureResp))
			}
			if tc.captureRaw != nil {
				opts = append(opts, httper.WithDestination(tc.captureRaw))
			}
			if tc.useJSONNumb {
				opts = append(opts, httper.WithJSONNumb())
			}

			if len(tc.path) > 0 {
				copied := *tc.url
				copied.Path = tc.path
				tc.url = &copied
			}

			req, err := httper.Request(t.Context(), tc.url, tc.method, reqOpts...)
			if err != nil {
				t.Fatalf("generating req: %v", err)
			}

			err = client.Do(req, tc.expStatus, opts...)
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
			url:         httper.URL("https", "localhost", "/", httper.WithPort(8888)),
			method:      http.MethodGet,
			payload:     nil,
			contentType: "",
			headers:     nil,
		},
		"withPayload": {
			url:         httper.URL("https", "localhost", "/", httper.WithPort(8888)),
			method:      http.MethodPost,
			payload:     &payload{Body: "hey there"},
			contentType: "",
			headers:     nil,
		},
		"withCustomContentType": {
			url:         httper.URL("https", "localhost", "/", httper.WithPort(8888)),
			method:      http.MethodGet,
			payload:     nil,
			contentType: "text/html",
			headers:     nil,
		},
		"withHeaders": {
			url:         httper.URL("https", "localhost", "/", httper.WithPort(8888)),
			method:      http.MethodPost,
			payload:     nil,
			contentType: "",
			headers: map[string][]string{
				"Single-Val": {"value"},
				"Multi-Val":  {"value", "value2"},
			},
		},
		"withSingleCookie": {
			url:    httper.URL("https", "localhost", "/", httper.WithPort(8888)),
			method: http.MethodGet,
			cookies: []*http.Cookie{
				{Name: "session", Value: "abc123"},
			},
		},
		"withMultipleCookies": {
			url:    httper.URL("https", "localhost", "/", httper.WithPort(8888)),
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
			var opts []httper.RequestOption
			if tc.payload != nil {
				opts = append(opts, httper.WithPayload(*tc.payload))
			}

			if len(tc.contentType) > 0 {
				opts = append(opts, httper.WithContentType(tc.contentType))
			}

			if tc.headers != nil {
				opts = append(opts, httper.WithHeaders(tc.headers))
			}

			if tc.cookies != nil {
				opts = append(opts, httper.WithCookies(tc.cookies...))
			}

			req, err := httper.Request(t.Context(), tc.url, tc.method, opts...)
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
			var opts []httper.URLOption
			if tc.qs != nil {
				opts = append(opts, httper.WithQueryStrings(tc.qs))
			}
			if tc.port != 0 {

				opts = append(opts, httper.WithPort(tc.port))
			}

			url := httper.URL(tc.scheme, tc.host, tc.path, opts...)

			if url.String() != tc.exp {
				t.Errorf("exp generated url:, %q, got: %q", tc.exp, url.String())
			}
		})
	}
}

const successRespBody = "success"

func mockServer(t *testing.T) *test {
	t.Helper()

	client, err := httper.New()
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
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
		Client:    client,
		server:    server,
		serverURL: testURL,
		teardown: func() {
			server.Close()
		},
	}

	return &ts
}
