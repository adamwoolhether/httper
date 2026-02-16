[![Go Reference (client)](https://pkg.go.dev/badge/github.com/adamwoolhether/httper/client.svg)](https://pkg.go.dev/github.com/adamwoolhether/httper/client)
[![Go Reference (web)](https://pkg.go.dev/badge/github.com/adamwoolhether/httper/web.svg)](https://pkg.go.dev/github.com/adamwoolhether/httper/web)
[![Build](https://github.com/adamwoolhether/httper/actions/workflows/config.yaml/badge.svg?branch=main)](https://github.com/adamwoolhether/httper/actions/workflows/config.yaml)

# httper
This is how I've been constructing HTTP services for a while now, or some variation thereof. Finally decided to make it into a reusable, shared repo.   
And now, the elevator pitch (to myself):

A lightweight Go toolkit for HTTP — ships as two independent modules:

- **`client`** — HTTP client wrapper with functional options, streaming file downloads, async batch downloads, and token-bucket rate limiting.
- **`web`** — Mux, middleware, server lifecycle, request/response helpers, and structured errors built on `net/http`.

Each module is versioned and imported independently — use one, the other, or both. The mux and server packages are also independent of each other: you can use the mux with your own server, or wrap any `http.Handler` with the server for signal-driven lifecycle management.

More often than not, HTTP clients and servers are highly use-case specific — I could never find myself to like other highly engineered solutions.
This is meant to be lightweight, a mere wrapper around the standard library.

## Contents

- [Install](#install)
- [Client](#client)
  - [Quick Start](#quick-start)
  - [Features](#features) — [JSON](#json-requests) | [Downloads](#file-downloads) | [Async](#async--batch-downloads) | [Rate Limiting](#rate-limiting)
  - [Client Options Reference](#client-options-reference)
- [Web](#web)
  - [Quick Start](#quick-start-1)
  - [Routing](#routing) — [Groups & Mounts](#groups--mounts)
  - [Server](#server)
  - [Middleware](#middleware)
  - [Request & Response Helpers](#request--response-helpers)
  - [Structured Errors](#structured-errors)
  - [Web Options Reference](#web-options-reference)

## Install

```sh
# Client module
go get github.com/adamwoolhether/httper/client

# Web module
go get github.com/adamwoolhether/httper/web
```

---

## Client

### Quick Start
```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/adamwoolhether/httper/client"
)

func main() {
	// Defaults to http defaults out of the box — optionally supply your own
	// http.Client or http.RoundTripper for full control.
	c, err := client.Build(
		client.WithTimeout(10 * time.Second),
		client.WithUserAgent("my-app/1.0"),
		// client.WithClient(customHTTPClient),
		// client.WithTransport(customRoundTripper),
	)
	if err != nil {
		log.Fatal(err)
	}

	u := client.URL("https", "httpbin.org", "/get")
	req, err := client.Request(context.Background(), u, http.MethodGet)
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

### Features

#### JSON Requests

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

#### File Downloads

Stream a file to disk with optional checksum verification and progress logging.
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

#### Async & Batch Downloads

Download multiple files concurrently with a bounded worker pool.
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

#### Rate Limiting

Wrap the transport with a token-bucket limiter.
See [Client Options](#client-options).

```go
c, err := client.Build(
	client.WithThrottle(10, 5), // 10 req/s, burst of 5
)
```

### Client Options Reference

#### Client Options

Passed to `client.Build(...)`.

```go
client.WithClient(hc)            // Replace the default http.Client
client.WithTransport(rt)         // Set a custom http.RoundTripper
client.WithTimeout(d)            // Set the overall request timeout
client.WithUserAgent(s)          // Add a persistent User-Agent header
client.WithThrottle(rps, burst)  // Enable token-bucket rate limiting
client.WithNoFollowRedirects()   // Prevent following HTTP redirects
client.WithLogger(l)             // Inject a custom slog.Logger
```

#### Request Options

Passed to `client.Request(...)`.

```go
client.WithPayload(body)      // Set the JSON-encoded request body
client.WithContentType(ct)    // Override the default "application/json" Content-Type
client.WithHeaders(h)         // Add custom headers to the request
client.WithCookies(c...)      // Attach cookies to the request
```

#### Do Options

Passed to `client.Do(...)`.

```go
client.WithDestination(&v)  // Decode the response body into v
client.WithJSONNumb()        // Preserve number precision as json.Number
```

#### URL Options

Passed to `client.URL(...)`.

```go
client.WithQueryStrings(kv)  // Append query parameters
client.WithPort(p)           // Set the port number on the host
```

#### Download Options

Passed to `client.Download(...)` / `client.DownloadAsync(...)`.

```go
download.WithBatch(n)              // Enable batch mode with bounded concurrency
download.WithChecksum(h, expected) // Verify file checksum after download
download.WithProgress()            // Enable periodic progress logging
download.WithSkipExisting()        // Skip download if the file already exists
```

---

## Web

### Quick Start
```go
package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/middleware"
	"github.com/adamwoolhether/httper/web/mux"
	"github.com/adamwoolhether/httper/web/server"
)

func main() {
	log := slog.Default()

	app := mux.New(mux.WithMiddleware(
		middleware.Logger(log),
		middleware.Errors(log),
		middleware.Panics(),
	))

	app.Get("/hello/{name}", func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		name, err := web.Param(r, "name")
		if err != nil {
			return err
		}
		return web.RespondJSON(ctx, w, http.StatusOK, map[string]string{"hello": name})
	})

	srv := server.New(app, server.WithHost(":3000"))
	if err := srv.Run(); err != nil {
		log.Error("server", "error", err)
	}
}
```

### Routing

The `mux` package wraps `net/http.ServeMux` with error-returning handlers, middleware chaining, and OpenTelemetry tracing.

```go
// Handler is an http.Handler that returns an error.
type Handler func(ctx context.Context, w http.ResponseWriter, r *http.Request) error

// Middleware wraps a Handler.
type Middleware func(handler Handler) Handler
```

Register routes with convenience methods:

```go
app.Get(path, handler, mw...)
app.Post(path, handler, mw...)
app.Put(path, handler, mw...)
app.Patch(path, handler, mw...)
app.Delete(path, handler, mw...)
```

Or use the lower-level methods directly:

```go
app.Handle(method, group, path, handler, mw...)      // full control
app.HandleRaw(method, group, path, httpHandler, mw...)// adapt a std http.Handler
app.HandleNoMiddleware(method, group, path, handler)  // skip all route middleware
```

#### Groups & Mounts

`Group()` shares the same ServeMux but gets an independent middleware stack.
`Mount(prefix)` scopes all routes under a URL prefix.

```go
api := app.Mount("api/v1")
api.Use(authMiddleware)

api.Get("/users", listUsers)
api.Post("/users", createUser)

// Public group — no auth middleware
pub := app.Group()
pub.Get("/health", healthCheck)
```

### Server

`server.New` wraps `net/http.Server` with signal-driven graceful shutdown.

**Defaults:** host `:8080`, read timeout 5s, write timeout 10s, idle timeout 120s, shutdown timeout 20s.

```go
srv := server.New(app,
	server.WithHost(":3000"),
	server.WithReadTimeout(10 * time.Second),
	server.WithShutdownFunc(func(ctx context.Context) error {
		log.Info("closing database")
		return db.Close()
	}),
)

// Run blocks until SIGINT/SIGTERM, then gracefully shuts down.
if err := srv.Run(); err != nil {
	log.Error("server", "error", err)
}
```

`Shutdown(ctx)` can also be called directly — the caller's context controls the deadline.

### Middleware

Pass middleware to `mux.WithMiddleware(...)` and they are automatically sorted by priority:

| Priority | Middleware | Scope  | Description                           |
|----------|------------|--------|---------------------------------------|
| 1        | `CORS`     | Global | Cross-origin resource sharing         |
| 2        | `CSRF`     | Global | Cross-site request forgery protection |
| 3        | `Logger`   | Route  | Request start/completion logging      |
| 4        | `Errors`   | Route  | Structured error responses            |
| 5        | *custom*   | Route  | Any user-supplied middleware           |
| 100      | `Panics`   | Route  | Panic recovery                        |

Global middleware runs on every request (via `ServeHTTP`). Route middleware runs per matched route.

```go
middleware.CORS(origins, headers...)   // []string origins, optional custom headers
middleware.CSRF(origins...)            // trusted origins (uses net/http.CrossOriginProtection)
middleware.Logger(log)                 // *slog.Logger
middleware.Errors(log)                 // *slog.Logger — catches *errs.Error and FieldErrors
middleware.Panics()                    // recovers from panics
```

Per-route middleware can also be added inline:

```go
app.Get("/admin", adminHandler, authMiddleware)
```

### Request & Response Helpers

**Path parameters:**
```go
web.Param(r, "id")        // string
web.ParamInt(r, "id")     // int
web.ParamInt64(r, "id")   // int64
```

**Query parameters:**
```go
web.QueryString(r, "q")   // string
web.QueryBool(r, "flag")  // bool
web.QueryInt(r, "page")   // int
web.QueryInt64(r, "ts")   // int64
```

**Decode & Respond:**
```go
web.Decode(r, &input)                        // JSON decode + validate
web.RespondJSON(ctx, w, statusCode, data)    // JSON response
web.RespondError(ctx, w, errsErr)            // structured error response
web.Redirect(w, r, url, code)               // HTTP redirect (3xx)
```

### Structured Errors

The `errs` package provides typed errors that map to HTTP status codes.

```go
errs.New(http.StatusNotFound, err)           // app-level error with status code
errs.NewInternal(err)                        // 500 — message hidden from clients
errs.NewFieldsError("email", err)            // field validation error
```

`FieldErrors` is a slice of `FieldError{Field, Err}` — the `Errors` middleware automatically responds with 422 when it encounters one.

### Web Options Reference

#### Mux Options

Passed to `mux.New(...)`.

```go
mux.WithMiddleware(mw...)             // Register middleware (auto-prioritized)
mux.WithTracer(tracer)                // Inject an OpenTelemetry tracer
mux.WithLogger(log)                   // Set the logger for internal errors
mux.WithStaticFS(fsys, pathPrefix)    // Serve static files from an fs.FS
```

#### Server Options

Passed to `server.New(handler, ...)`.

```go
server.WithServer(srv)                // Inject an existing *http.Server as base
server.WithHost(addr)                 // Listen address (default ":8080")
server.WithReadTimeout(d)             // Read timeout (default 5s)
server.WithWriteTimeout(d)            // Write timeout (default 10s)
server.WithIdleTimeout(d)             // Idle timeout (default 120s)
server.WithShutdownTimeout(d)         // Shutdown timeout for Run (default 20s)
server.WithLogger(log)                // Lifecycle logger
server.WithShutdownFunc(fn)           // Register a shutdown hook
server.WithTLS(certFile, keyFile)     // Enable TLS
```

---

## Thanks
This grew over the years, originally based on knowledge obtained from [Powerful Command-Line Applications in Go](https://pragprog.com/titles/rggo/powerful-command-line-applications-in-go/) by [Ricardo Gerardi](https://github.com/rgerardi).
And of course a shoutout to [Ardan Labs](https://github.com/ardanlabs), who has inspired many decisions here.
