[![Go Reference](https://pkg.go.dev/badge/github.com/adamwoolhether/httper.svg)](https://pkg.go.dev/github.com/adamwoolhether/httper)
[![Build](https://github.com/adamwoolhether/httper/actions/workflows/config.yaml/badge.svg?branch=main)](https://github.com/adamwoolhether/httper/actions/workflows/config.yaml)

# httper

A *simple* Go HTTP client wrapper with functional options, streaming file downloads, async batch downloads, and token-bucket rate limiting.  

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
	c, err := httper.NewClient(
		client.WithTimeout(10 * time.Second),
		client.WithUserAgent("my-app/1.0"),
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

## Features

### JSON Requests

Build a request with a JSON payload and decode the response into a struct.
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

Stream a file to disk with optional checksum verification and progress logging.
See [Download Options](#download-options).

```go
import "crypto/sha256"

u := client.URL("https", "example.com", "/archive.tar.gz")
req, _ := client.Request(ctx, u, http.MethodGet)

err := c.Download(req, http.StatusOK, "/tmp/archive.tar.gz",
	client.WithChecksum(sha256.New(), "e3b0c44298fc1c14..."),
	client.WithProgress(),
	client.WithSkipExisting(),
)
```

### Async & Batch Downloads

Download multiple files concurrently with a bounded worker pool.
See [Download Options](#download-options).

```go
u1 := client.URL("https", "example.com", "/file1.zip")
req1, _ := client.Request(ctx, u1, http.MethodGet)

result, err := c.DownloadAsync(req1, http.StatusOK, "/tmp/file1.zip",
	client.WithBatch(4), // max 4 concurrent downloads
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

Wrap the transport with a token-bucket limiter.
See [Client Options](#client-options).

```go
c, err := httper.NewClient(
	client.WithThrottle(10, 5), // 10 req/s, burst of 5
)
```