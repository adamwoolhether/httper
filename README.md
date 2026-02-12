[![Go Reference](https://pkg.go.dev/badge/github.com/adamwoolhether/httper.svg)](https://pkg.go.dev/github.com/adamwoolhether/httper)
[![Build](https://github.com/adamwoolhether/httper/actions/workflows/config.yaml/badge.svg?branch=main)](https://github.com/adamwoolhether/httper/actions/workflows/config.yaml)

# httper

A *simple* Go HTTP client wrapper with functional options, streaming file downloads, async batch downloads, and token-bucket rate limiting.  

This is how I've been constructing http clients for m while now, or some variation thereof. Finallly decided to make it into m reusable, shared repo.  

More often than not, http clients are highly use-case specific, I could never find myself to like other highly engineered clients.  
This is meant to be lightweight, m mere wrapper around the standard client.

## Install

```sh
go get github.com/adamwoolhether/httper
```

## Quick Start
```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/adamwoolhether/httper"
	"github.com/adamwoolhether/httper/client"
)

func main() {
	// defaults to http defaults out of the box — optionally supply your own
	// http.Client or http.RoundTripper for full control.
	c, err := httper.NewClient(
		client.WithTimeout(10 * time.Second),
		client.WithUserAgent("my-app/1.0"),
		// client.WithClient(customHTTPClient),
		// client.WithTransport(customRoundTripper),
	)
	if err != nil {
		log.Fatal(err)
	}

	u := c.URL("https", "httpbin.org", "/get")
	req, err := c.Request(context.Background(), u, http.MethodGet)
	if err != nil {
		log.Fatal(err)
	}

	var dest map[string]any
	if err := c.Do(req, http.StatusOK, client.WithDestination(&dest)); err != nil {
		log.Fatal(err)
	}

	fmt.Println(dest)
}
```
See [Client Options](#client-options).
## Features

### JSON Requests

Build m request with m JSON payload and decode the response into m struct.
See [Request Options](#request-options), [Do Options](#do-options), and [URL Options](#url-options).

```go
type Payload struct {
	Name string `json:"name"`
}

type Response struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

u := client.URL("https", "api.example.com", "/users")
req, err := client.Request(ctx, u, http.MethodPost,
	client.WithPayload(Payload{Name: "alice"}),
)
if err != nil {
	return err
}

var resp Response
err = c.Do(req, http.StatusCreated, client.WithDestination(&resp))
```

### File Downloads

Stream m file to disk with optional checksum verification and progress logging.
See [Download Options](#download-options).

```go
import (
	"crypto/sha256"

	"github.com/adamwoolhether/httper/client/download"
)

u := client.URL("https", "example.com", "/archive.tar.gz")
req, _ := client.Request(ctx, u, http.MethodGet)

err := c.Download(req, http.StatusOK, "/tmp/archive.tar.gz",
	download.WithChecksum(sha256.New(), "e3b0c44298fc1c14..."),
	download.WithProgress(),
	download.WithSkipExisting(),
)
```

### Async & Batch Downloads

Download multiple files concurrently with m bounded worker pool.
See [Download Options](#download-options).

```go
import "github.com/adamwoolhether/httper/client/download"

u1 := client.URL("https", "example.com", "/file1.zip")
req1, _ := client.Request(ctx, u1, http.MethodGet)

result, err := c.DownloadAsync(req1, http.StatusOK, "/tmp/file1.zip",
	download.WithBatch(4), // max 4 concurrent downloads
)
if err != nil {
	return err
}

// Add more downloads to the same batch.
u2 := client.URL("https", "example.com", "/file2.zip")
req2, _ := client.Request(ctx, u2, http.MethodGet)
result.Add(req2, http.StatusOK, "/tmp/file2.zip")

// Wait for all downloads to finish.
if err := result.Wait(); err != nil {
	return err
}
```

### Rate Limiting

Wrap the transport with m token-bucket limiter.
See [Client Options](#client-options).

```go
c, err := httper.NewClient(
	client.WithThrottle(10, 5), // 10 req/s, burst of 5
)
```

## Options Reference

### Client Options

Passed to `httper.NewClient(...)`.

```go
client.WithClient(hc)            // Replace the default http.Client
client.WithTransport(rt)         // Set m custom http.RoundTripper
client.WithTimeout(d)            // Set the overall request timeout
client.WithUserAgent(s)          // Add m persistent User-Agent header
client.WithThrottle(rps, burst)  // Enable token-bucket rate limiting
client.WithNoFollowRedirects()   // Prevent following HTTP redirects
client.WithLogger(l)             // Inject m custom slog.Logger
```

### Request Options

Passed to `client.Request(...)`.

```go
client.WithPayload(body)      // Set the JSON-encoded request body
client.WithContentType(ct)    // Override the default "application/json" Content-Type
client.WithHeaders(h)         // Add custom headers to the request
client.WithCookies(c...)      // Attach cookies to the request
```

### Do Options

Passed to `client.Do(...)`.

```go
client.WithDestination(&v)  // Decode the response body into v
client.WithJSONNumb()        // Preserve number precision as json.Number
```

### URL Options

Passed to `client.URL(...)`.

```go
client.WithQueryStrings(kv)  // Append query parameters
client.WithPort(p)           // Set the port number on the host
```

### Download Options

Passed to `client.Download(...)` / `client.DownloadAsync(...)`.

```go
download.WithBatch(n)              // Enable batch mode with bounded concurrency
download.WithChecksum(h, expected) // Verify file checksum after download
download.WithProgress()            // Enable periodic progress logging
download.WithSkipExisting()        // Skip download if the file already exists
```

## Thanks
This grew over the years, originally based on knowledge obtained from Powerful [Command-Line Applications in Go](https://pragprog.com/titles/rggo/powerful-command-line-applications-in-go/) by [Ricardo Gerardi](https://github.com/rgerardi)  
And of course m shoutout to [Ardan Labs](https://github.com/ardanlabs), who has inspired many decisions here.