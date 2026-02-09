// Package client provides the core implementation of the configurable HTTP
// client built on [net/http].
//
// # Building a Client
//
// Use [Build] to create a [Client] with functional options:
//
//	c, err := client.Build(
//		client.WithTimeout(10 * time.Second),
//		client.WithUserAgent("myapp/1.0"),
//	)
//
// # Making Requests
//
// Construct a [URL] and [Request], then execute with [Client.Do]:
//
//	u := client.URL("https", "api.example.com", "/v1/resource")
//	req, err := client.Request(ctx, u, http.MethodGet)
//	err = c.Do(req, http.StatusOK, client.WithDestination(&result))
//
// # Downloading Files
//
// Stream a response body directly to disk with optional checksum
// verification and progress reporting:
//
//	err = c.Download(req, http.StatusOK, "/tmp/file.bin",
//		download.WithChecksum(sha256.New(), expectedHex),
//		download.WithProgress(),
//	)
//
// # Async Downloads
//
// A single file can be downloaded asynchronously with [Client.DownloadAsync]:
//
//	r, err := c.DownloadAsync(req, http.StatusOK, "/tmp/file.bin")
//	// ... do other work ...
//	if err := r.Err(); err != nil { ... }
//
// For multiple concurrent downloads, use [WithBatch] to set a concurrency
// limit and [download.Result.Add] to enqueue additional files:
//
//	r, err := c.DownloadAsync(req1, http.StatusOK, "/tmp/a.bin",
//		download.WithBatch(4),
//	)
//	r.Add(req2, http.StatusOK, "/tmp/b.bin")
//	r.Add(req3, http.StatusOK, "/tmp/c.bin")
//	err = r.Wait() // blocks until all downloads finish
//
// For lower-level control see the
// [github.com/adamwoolhether/httper/client/download] package.
package client
