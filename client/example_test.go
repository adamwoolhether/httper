package client_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/adamwoolhether/httper/client"
	"github.com/adamwoolhether/httper/client/download"
)

func ExampleBuild() {
	c, err := client.Build(
		client.WithTimeout(10*time.Second),
		client.WithUserAgent("example/1.0"),
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	_ = c
	fmt.Println("client built")
	// Output: client built
}

func ExampleURL() {
	u := client.URL("https", "example.com", "/api/v1",
		client.WithPort(8443),
		client.WithQueryStrings(map[string]string{"key": "value"}),
	)

	fmt.Println(u.String())
	// Output: https://example.com:8443/api/v1?key=value
}

func ExampleRequest() {
	type payload struct {
		Name string `json:"name"`
	}

	u := client.URL("https", "example.com", "/users")

	req, err := client.Request(context.Background(), u, http.MethodPost,
		client.WithPayload(payload{Name: "alice"}),
		client.WithHeaders(map[string][]string{"X-Request-ID": {"abc123"}}),
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(req.Method, req.URL.Path)
	// Output: POST /users
}

func ExampleClient_Do() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer ts.Close()

	c, _ := client.Build()
	u, _ := url.Parse(ts.URL)
	req, _ := client.Request(context.Background(), u, http.MethodGet)

	var resp struct{ Status string }
	if err := c.Do(req, http.StatusOK, client.WithDestination(&resp)); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(resp.Status)
	// Output: ok
}

func ExampleClient_Request() {
	c, _ := client.Build()

	u := c.URL("https", "example.com", "/api/v1/users")

	req, err := c.Request(context.Background(), u, http.MethodGet)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(req.Method, req.URL)
	// Output: GET https://example.com/api/v1/users
}

func ExampleClient_URL() {
	c, _ := client.Build()

	u := c.URL("https", "example.com", "/search",
		client.WithPort(8443),
		client.WithQueryStrings(map[string]string{"q": "test"}),
	)

	fmt.Println(u.String())
	// Output: https://example.com:8443/search?q=test
}

func ExampleClient_Download() {
	body := []byte("file contents")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer ts.Close()

	c, _ := client.Build()
	u, _ := url.Parse(ts.URL)
	req, _ := client.Request(context.Background(), u, http.MethodGet)

	dest := filepath.Join(os.TempDir(), "httper-example-dl.bin")
	defer os.Remove(dest)

	if err := c.Download(req, http.StatusOK, dest, download.WithProgress()); err != nil {
		fmt.Println("error:", err)
		return
	}

	data, _ := os.ReadFile(dest)
	fmt.Println(string(data))
	// Output: file contents
}

func ExampleClient_DownloadAsync() {
	body := []byte("async file contents")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer ts.Close()

	c, _ := client.Build()
	u, _ := url.Parse(ts.URL)
	req, _ := client.Request(context.Background(), u, http.MethodGet)

	dest := filepath.Join(os.TempDir(), "httper-example-async-dl.bin")
	defer os.Remove(dest)

	r, err := c.DownloadAsync(req, http.StatusOK, dest)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	// Block until the download completes.
	if err := r.Err(); err != nil {
		fmt.Println("download error:", err)
		return
	}

	data, _ := os.ReadFile(dest)
	fmt.Println(string(data))
	// Output: async file contents
}

func ExampleClient_DownloadAsync_batch() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := []byte("file:" + r.URL.Path)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	c, _ := client.Build()

	destA := filepath.Join(os.TempDir(), "httper-example-batch-a.bin")
	destB := filepath.Join(os.TempDir(), "httper-example-batch-b.bin")
	defer os.Remove(destA)
	defer os.Remove(destB)

	uA, _ := url.Parse(ts.URL + "/a")
	reqA, _ := client.Request(context.Background(), uA, http.MethodGet)

	// Start the first download with a batch concurrency limit of 2.
	r, err := c.DownloadAsync(reqA, http.StatusOK, destA, download.WithBatch(2))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	// Enqueue a second download into the same batch.
	uB, _ := url.Parse(ts.URL + "/b")
	reqB, _ := client.Request(context.Background(), uB, http.MethodGet)
	r.Add(reqB, http.StatusOK, destB)

	// Wait for all downloads to complete.
	if err := r.Wait(); err != nil {
		fmt.Println("batch error:", err)
		return
	}

	dataA, _ := os.ReadFile(destA)
	dataB, _ := os.ReadFile(destB)
	fmt.Println(string(dataA))
	fmt.Println(string(dataB))
	// Output:
	// file:/a
	// file:/b
}

// ————————————————————————————————————————————————————————————————————
// Build option examples
// ————————————————————————————————————————————————————————————————————

func ExampleWithClient() {
	custom := &http.Client{Timeout: 30 * time.Second}

	c, err := client.Build(client.WithClient(custom))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	_ = c
	fmt.Println("ok")
	// Output: ok
}

func ExampleWithTransport() {
	transport := &http.Transport{
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	}

	c, err := client.Build(client.WithTransport(transport))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	_ = c
	fmt.Println("ok")
	// Output: ok
}

func ExampleWithTimeout() {
	c, err := client.Build(client.WithTimeout(5 * time.Second))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	_ = c
	fmt.Println("ok")
	// Output: ok
}

func ExampleWithUserAgent() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ua":"`+r.Header.Get("User-Agent")+`"}`)
	}))
	defer ts.Close()

	c, _ := client.Build(client.WithUserAgent("myapp/1.0"))
	u, _ := url.Parse(ts.URL)
	req, _ := client.Request(context.Background(), u, http.MethodGet)

	var resp struct{ UA string }
	c.Do(req, http.StatusOK, client.WithDestination(&resp))
	fmt.Println(resp.UA)
	// Output: myapp/1.0
}

func ExampleWithThrottle() {
	c, err := client.Build(client.WithThrottle(10, 5))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	_ = c
	fmt.Println("ok")
	// Output: ok
}

func ExampleWithNoFollowRedirects() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/other", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c, _ := client.Build(client.WithNoFollowRedirects())
	u, _ := url.Parse(ts.URL)
	req, _ := client.Request(context.Background(), u, http.MethodGet)

	// Without NoFollowRedirects the client would follow the 302.
	// With it, we get the redirect status directly.
	err := c.Do(req, http.StatusFound)
	fmt.Println("error:", err)
	// Output: error: <nil>
}

func ExampleWithLogger() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	c, err := client.Build(client.WithLogger(logger))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	_ = c
	fmt.Println("ok")
	// Output: ok
}

// ————————————————————————————————————————————————————————————————————
// Do option examples
// ————————————————————————————————————————————————————————————————————

func ExampleWithDestination() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"name":"alice","age":30}`)
	}))
	defer ts.Close()

	c, _ := client.Build()
	u, _ := url.Parse(ts.URL)
	req, _ := client.Request(context.Background(), u, http.MethodGet)

	var user struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	if err := c.Do(req, http.StatusOK, client.WithDestination(&user)); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(user.Name, user.Age)
	// Output: alice 30
}

func ExampleWithJSONNumb() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":9007199254740993}`)
	}))
	defer ts.Close()

	c, _ := client.Build()
	u, _ := url.Parse(ts.URL)
	req, _ := client.Request(context.Background(), u, http.MethodGet)

	// UseNumber preserves large integers that would lose precision as float64.
	var resp map[string]any
	if err := c.Do(req, http.StatusOK, client.WithDestination(&resp), client.WithJSONNumb()); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(resp["id"])
	// Output: 9007199254740993
}

// ————————————————————————————————————————————————————————————————————
// Request option examples
// ————————————————————————————————————————————————————————————————————

func ExampleWithPayload() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"received":"`+body["msg"]+`"}`)
	}))
	defer ts.Close()

	c, _ := client.Build()
	u, _ := url.Parse(ts.URL)

	req, _ := client.Request(context.Background(), u, http.MethodPost,
		client.WithPayload(map[string]string{"msg": "hello"}),
	)

	var resp struct{ Received string }
	c.Do(req, http.StatusOK, client.WithDestination(&resp))
	fmt.Println(resp.Received)
	// Output: hello
}

func ExampleWithContentType() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ct":"`+r.Header.Get("Content-Type")+`"}`)
	}))
	defer ts.Close()

	c, _ := client.Build()
	u, _ := url.Parse(ts.URL)

	req, _ := client.Request(context.Background(), u, http.MethodPost,
		client.WithContentType("application/xml"),
	)

	var resp struct{ CT string }
	c.Do(req, http.StatusOK, client.WithDestination(&resp))
	fmt.Println(resp.CT)
	// Output: application/xml
}

func ExampleWithHeaders() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"rid":"`+r.Header.Get("X-Request-ID")+`"}`)
	}))
	defer ts.Close()

	c, _ := client.Build()
	u, _ := url.Parse(ts.URL)

	req, _ := client.Request(context.Background(), u, http.MethodGet,
		client.WithHeaders(map[string][]string{
			"X-Request-ID": {"req-123"},
		}),
	)

	var resp struct{ RID string }
	c.Do(req, http.StatusOK, client.WithDestination(&resp))
	fmt.Println(resp.RID)
	// Output: req-123
}

func ExampleWithCookies() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"session":"`+cookie.Value+`"}`)
	}))
	defer ts.Close()

	c, _ := client.Build()
	u, _ := url.Parse(ts.URL)

	req, _ := client.Request(context.Background(), u, http.MethodGet,
		client.WithCookies(&http.Cookie{Name: "session", Value: "abc123"}),
	)

	var resp struct{ Session string }
	c.Do(req, http.StatusOK, client.WithDestination(&resp))
	fmt.Println(resp.Session)
	// Output: abc123
}

// ————————————————————————————————————————————————————————————————————
// URL option examples
// ————————————————————————————————————————————————————————————————————

func ExampleWithQueryStrings() {
	u := client.URL("https", "example.com", "/search",
		client.WithQueryStrings(map[string]string{
			"q":    "golang",
			"page": "1",
		}),
	)

	// Parse the query back to verify both params are present.
	q := u.Query()
	fmt.Println("q:", q.Get("q"))
	fmt.Println("page:", q.Get("page"))
	// Output:
	// q: golang
	// page: 1
}

func ExampleWithPort() {
	u := client.URL("https", "example.com", "/api",
		client.WithPort(9090),
	)

	fmt.Println(u.Host)
	// Output: example.com:9090
}

// ————————————————————————————————————————————————————————————————————
// Download option examples
// ————————————————————————————————————————————————————————————————————

func ExampleWithChecksum() {
	body := []byte("verified content")
	sum := sha256.Sum256(body)
	expectedHex := hex.EncodeToString(sum[:])

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer ts.Close()

	c, _ := client.Build()
	u, _ := url.Parse(ts.URL)
	req, _ := client.Request(context.Background(), u, http.MethodGet)

	dest := filepath.Join(os.TempDir(), "httper-example-checksum.bin")
	defer os.Remove(dest)

	err := c.Download(req, http.StatusOK, dest,
		download.WithChecksum(sha256.New(), expectedHex),
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	data, _ := os.ReadFile(dest)
	fmt.Println(string(data))
	// Output: verified content
}

func ExampleWithProgress() {
	body := []byte("progress content")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer ts.Close()

	c, _ := client.Build()
	u, _ := url.Parse(ts.URL)
	req, _ := client.Request(context.Background(), u, http.MethodGet)

	dest := filepath.Join(os.TempDir(), "httper-example-progress.bin")
	defer os.Remove(dest)

	// Progress logs are emitted via the client's logger.
	err := c.Download(req, http.StatusOK, dest, download.WithProgress())
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	data, _ := os.ReadFile(dest)
	fmt.Println(string(data))
	// Output: progress content
}

func ExampleWithSkipExisting() {
	body := []byte("original")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer ts.Close()

	c, _ := client.Build()
	u, _ := url.Parse(ts.URL)

	dest := filepath.Join(os.TempDir(), "httper-example-skip.bin")
	defer os.Remove(dest)

	// First download creates the file.
	req1, _ := client.Request(context.Background(), u, http.MethodGet)
	c.Download(req1, http.StatusOK, dest)

	// Second download with WithSkipExisting skips because the file exists.
	req2, _ := client.Request(context.Background(), u, http.MethodGet)
	err := c.Download(req2, http.StatusOK, dest, download.WithSkipExisting())

	fmt.Println("error:", err)
	data, _ := os.ReadFile(dest)
	fmt.Println(string(data))
	// Output:
	// error: <nil>
	// original
}

func ExampleWithBatch() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := []byte("batch:" + r.URL.Path)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	c, _ := client.Build()

	dest := filepath.Join(os.TempDir(), "httper-example-withbatch.bin")
	defer os.Remove(dest)

	u, _ := url.Parse(ts.URL + "/file")
	req, _ := client.Request(context.Background(), u, http.MethodGet)

	// WithBatch(4) creates a queue allowing 4 concurrent downloads.
	r, err := c.DownloadAsync(req, http.StatusOK, dest, download.WithBatch(4))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	if err := r.Err(); err != nil {
		fmt.Println("download error:", err)
		return
	}

	data, _ := os.ReadFile(dest)
	fmt.Println(string(data))
	// Output: batch:/file
}
