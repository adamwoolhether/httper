//go:build integration

package e2e_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/adamwoolhether/httper/client"
	"github.com/adamwoolhether/httper/client/download"
	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/errs"
	"github.com/adamwoolhether/httper/web/middleware"
	"github.com/adamwoolhether/httper/web/mux"
)

type user struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

type itemResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type typedItemResp struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type queryResp struct {
	Search string `json:"search"`
	Page   string `json:"page"`
}

type typedQueryResp struct {
	Search  string `json:"search"`
	Page    int    `json:"page"`
	Enabled bool   `json:"enabled"`
}

type validateReq struct {
	Name  string `json:"name"  validate:"required"`
	Email string `json:"email" validate:"required,email"`
}

type headerEcho struct {
	UserAgent   string `json:"user_agent"`
	ContentType string `json:"content_type"`
	Custom      string `json:"custom"`
}

type cookieEcho struct {
	Session string `json:"session"`
	Token   string `json:"token"`
}

type numberResp struct {
	Price json.Number `json:"price"`
	Count json.Number `json:"count"`
}

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()

	level := slog.LevelError
	if os.Getenv("VERBOSE") != "" {
		level = slog.LevelInfo
	}

	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func newTestApp(t *testing.T) string {
	t.Helper()

	log := testLogger(t)

	app := mux.New(
		mux.WithMiddleware(
			middleware.CORS([]string{"https://allowed.example.com"}),
			middleware.Logger(log),
			middleware.Errors(log),
			middleware.Panics(),
		),
		mux.WithLogger(log),
	)

	registerRoutes(app)

	srv := httptest.NewServer(app)
	t.Cleanup(srv.Close)

	return srv.URL
}

func newFullTestApp(t *testing.T) string {
	t.Helper()

	log := testLogger(t)

	staticFS := fstest.MapFS{
		"index.html":         {Data: []byte("<html><body>hello</body></html>")},
		"assets/style.css":   {Data: []byte("body { color: red; }")},
		"assets/favicon.ico": {Data: []byte("icon-data")},
	}

	app := mux.New(
		mux.WithMiddleware(
			middleware.CORS([]string{"https://allowed.example.com"}),
			middleware.Logger(log),
			middleware.Errors(log),
			middleware.Panics(),
		),
		mux.WithLogger(log),
		mux.WithStaticFS(staticFS, "/static/"),
	)

	registerRoutes(app)
	registerExtraRoutes(app, log)

	srv := httptest.NewServer(app)
	t.Cleanup(srv.Close)

	return srv.URL
}

func registerRoutes(app *mux.App) {
	app.Post("/echo", echoHandler)
	app.Get("/items/{id}/{name}", itemHandler)
	app.Get("/query", queryHandler)
	app.Get("/error/not-found", notFoundHandler)
	app.Post("/validate", validateHandler)
	app.Get("/download", downloadHandler)
}

func registerExtraRoutes(app *mux.App, log *slog.Logger) {
	// Typed parameter routes.
	app.Get("/typed-item/{id}", typedItemHandler)
	app.Get("/typed-query", typedQueryHandler)

	// Header/cookie echo.
	app.Get("/echo-headers", echoHeadersHandler)
	app.Get("/echo-cookies", echoCookiesHandler)

	// No content.
	app.Delete("/resource/{id}", noContentHandler)

	// Redirect.
	app.Get("/redirect", redirectHandler)
	app.Get("/redirect-target", redirectTargetHandler)

	// Internal error (message should be obscured).
	app.Get("/error/internal", internalErrorHandler)

	// Auth failure.
	app.Get("/error/unauthorized", unauthorizedHandler)
	app.Get("/error/forbidden", forbiddenHandler)

	// Panic.
	app.Get("/panic", panicHandler)

	// Decode unknown fields.
	app.Post("/strict-decode", strictDecodeHandler)
	app.Post("/loose-decode", looseDecodeHandler)

	// Large numbers (JSON Number).
	app.Get("/numbers", numbersHandler)

	// All HTTP methods.
	app.Put("/resource", putHandler)
	app.Patch("/resource/{id}", patchHandler)

	// Mount sub-route.
	api := app.Mount("/api/v1")
	api.Get("/status", statusHandler)
	api.Post("/items", echoHandler)

	// Group with per-route middleware.
	admin := app.Group()
	admin.Use(requireAuthMiddleware)
	admin.Get("/admin/dashboard", dashboardHandler)

	// HandleRaw with std-lib handler.
	app.HandleRaw(http.MethodGet, "", "/raw/health", http.HandlerFunc(rawHealthHandler))
}

func newClient(t *testing.T) *client.Client {
	t.Helper()

	c, err := client.Build()
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	return c
}

func mustParseURL(t *testing.T, base, path string) *url.URL {
	t.Helper()

	u, err := url.Parse(base + path)
	if err != nil {
		t.Fatalf("parsing URL %s%s: %v", base, path, err)
	}

	return u
}

func splitBaseURL(t *testing.T, baseURL string) (string, string) {
	t.Helper()

	u, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parsing base URL: %v", err)
	}

	return u.Scheme, u.Host
}

func echoHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	var u user
	if err := web.Decode(r, &u); err != nil {
		return err
	}

	return web.RespondJSON(ctx, w, http.StatusCreated, u)
}

func itemHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	id, err := web.Param(r, "id")
	if err != nil {
		return errs.New(http.StatusBadRequest, err)
	}

	name, err := web.Param(r, "name")
	if err != nil {
		return errs.New(http.StatusBadRequest, err)
	}

	return web.RespondJSON(ctx, w, http.StatusOK, itemResp{
		ID:   id,
		Name: name,
	})
}

func typedItemHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	id, err := web.ParamInt(r, "id")
	if err != nil {
		return errs.New(http.StatusBadRequest, err)
	}

	return web.RespondJSON(ctx, w, http.StatusOK, typedItemResp{
		ID:   id,
		Name: "typed",
	})
}

func queryHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	search, err := web.QueryString(r, "search")
	if err != nil {
		return errs.New(http.StatusBadRequest, err)
	}

	page, err := web.QueryString(r, "page")
	if err != nil {
		return errs.New(http.StatusBadRequest, err)
	}

	return web.RespondJSON(ctx, w, http.StatusOK, queryResp{
		Search: search,
		Page:   page,
	})
}

func typedQueryHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	search, err := web.QueryString(r, "search")
	if err != nil {
		return errs.New(http.StatusBadRequest, err)
	}

	page, err := web.QueryInt(r, "page")
	if err != nil {
		return errs.New(http.StatusBadRequest, err)
	}

	enabled, err := web.QueryBool(r, "enabled")
	if err != nil {
		return errs.New(http.StatusBadRequest, err)
	}

	return web.RespondJSON(ctx, w, http.StatusOK, typedQueryResp{
		Search:  search,
		Page:    page,
		Enabled: enabled,
	})
}

func echoHeadersHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	return web.RespondJSON(ctx, w, http.StatusOK, headerEcho{
		UserAgent:   r.Header.Get("User-Agent"),
		ContentType: r.Header.Get("Content-Type"),
		Custom:      r.Header.Get("X-Custom-Header"),
	})
}

func echoCookiesHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	session, _ := r.Cookie("session")
	token, _ := r.Cookie("token")

	resp := cookieEcho{}
	if session != nil {
		resp.Session = session.Value
	}
	if token != nil {
		resp.Token = token.Value
	}

	return web.RespondJSON(ctx, w, http.StatusOK, resp)
}

func notFoundHandler(_ context.Context, _ http.ResponseWriter, _ *http.Request) error {
	return errs.New(http.StatusNotFound, fmt.Errorf("widget not found"))
}

func validateHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	var v validateReq
	if err := web.Decode(r, &v); err != nil {
		return err
	}

	return web.RespondJSON(ctx, w, http.StatusOK, v)
}

func downloadHandler(_ context.Context, w http.ResponseWriter, _ *http.Request) error {
	data := []byte("hello, this is test download content!")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(data)

	return err
}

func noContentHandler(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
	return web.RespondJSON(ctx, w, http.StatusNoContent, nil)
}

func redirectHandler(_ context.Context, w http.ResponseWriter, r *http.Request) error {
	return web.Redirect(w, r, "/redirect-target", http.StatusFound)
}

func redirectTargetHandler(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
	return web.RespondJSON(ctx, w, http.StatusOK, map[string]string{"arrived": "true"})
}

func internalErrorHandler(_ context.Context, _ http.ResponseWriter, _ *http.Request) error {
	return errs.NewInternal(fmt.Errorf("database connection failed: secret-dsn"))
}

func unauthorizedHandler(_ context.Context, _ http.ResponseWriter, _ *http.Request) error {
	return errs.New(http.StatusUnauthorized, fmt.Errorf("invalid token"))
}

func forbiddenHandler(_ context.Context, _ http.ResponseWriter, _ *http.Request) error {
	return errs.New(http.StatusForbidden, fmt.Errorf("access denied"))
}

func panicHandler(_ context.Context, _ http.ResponseWriter, _ *http.Request) error {
	panic("something went terribly wrong")
}

func strictDecodeHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	var u user
	if err := web.Decode(r, &u); err != nil {
		return errs.New(http.StatusBadRequest, err)
	}

	return web.RespondJSON(ctx, w, http.StatusOK, u)
}

func looseDecodeHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	var u user
	if err := web.DecodeAllowUnknownFields(r, &u); err != nil {
		return errs.New(http.StatusBadRequest, err)
	}

	return web.RespondJSON(ctx, w, http.StatusOK, u)
}

func numbersHandler(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
	// Use json.RawMessage to preserve exact numeric representation.
	raw := json.RawMessage(`{"price":99999999999999999.99,"count":12345678901234567}`)

	w.Header().Set("Content-Type", "application/json")
	mux.SetStatusCode(ctx, http.StatusOK)
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(raw)

	return err
}

func putHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	var u user
	if err := web.Decode(r, &u); err != nil {
		return err
	}

	return web.RespondJSON(ctx, w, http.StatusOK, u)
}

func patchHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	id, err := web.Param(r, "id")
	if err != nil {
		return errs.New(http.StatusBadRequest, err)
	}

	var u user
	if err := web.Decode(r, &u); err != nil {
		return err
	}

	return web.RespondJSON(ctx, w, http.StatusOK, map[string]any{
		"id":   id,
		"name": u.Name,
	})
}

func statusHandler(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
	return web.RespondJSON(ctx, w, http.StatusOK, map[string]string{"status": "ok"})
}

func dashboardHandler(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
	return web.RespondJSON(ctx, w, http.StatusOK, map[string]string{"page": "dashboard"})
}

// requireAuthMiddleware is a per-group middleware that checks for an auth header.
func requireAuthMiddleware(handler mux.Handler) mux.Handler {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-Auth-Token") == "" {
			return errs.New(http.StatusUnauthorized, fmt.Errorf("missing auth token"))
		}

		return handler(ctx, w, r)
	}
}

func rawHealthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

// =========================================================================
// Tests — Request/Response Cycle
// =========================================================================

func TestE2E_JSONRoundTrip(t *testing.T) {
	baseURL := newTestApp(t)
	c := newClient(t)

	sent := user{Name: "Alice", Email: "alice@test.com", Age: 30}

	reqURL := mustParseURL(t, baseURL, "/echo")
	req, err := c.Request(context.Background(), reqURL, http.MethodPost, client.WithPayload(sent))
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	var got user
	if err := c.Do(req, http.StatusCreated, client.WithDestination(&got)); err != nil {
		t.Fatalf("executing request: %v", err)
	}

	if got != sent {
		t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", got, sent)
	}
}

func TestE2E_PutMethod(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	sent := user{Name: "Updated", Email: "updated@test.com", Age: 40}

	reqURL := mustParseURL(t, baseURL, "/resource")
	req, err := c.Request(context.Background(), reqURL, http.MethodPut, client.WithPayload(sent))
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	var got user
	if err := c.Do(req, http.StatusOK, client.WithDestination(&got)); err != nil {
		t.Fatalf("executing request: %v", err)
	}

	if got != sent {
		t.Errorf("put mismatch:\n  got:  %+v\n  want: %+v", got, sent)
	}
}

func TestE2E_PatchMethod(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	payload := user{Name: "Patched"}

	reqURL := mustParseURL(t, baseURL, "/resource/77")
	req, err := c.Request(context.Background(), reqURL, http.MethodPatch, client.WithPayload(payload))
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	var got map[string]any
	if err := c.Do(req, http.StatusOK, client.WithDestination(&got)); err != nil {
		t.Fatalf("executing request: %v", err)
	}

	if got["id"] != "77" {
		t.Errorf("id = %v, want %q", got["id"], "77")
	}
	if got["name"] != "Patched" {
		t.Errorf("name = %v, want %q", got["name"], "Patched")
	}
}

func TestE2E_DeleteNoContent(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/resource/99")
	req, err := c.Request(context.Background(), reqURL, http.MethodDelete)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	if err := c.Do(req, http.StatusNoContent); err != nil {
		t.Fatalf("expected 204 No Content, got error: %v", err)
	}
}

func TestE2E_JSONNumber(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/numbers")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	var got numberResp
	if err := c.Do(req, http.StatusOK, client.WithDestination(&got), client.WithJSONNumb()); err != nil {
		t.Fatalf("executing request: %v", err)
	}

	// WithJSONNumb preserves exact numeric representation.
	if got.Price.String() != "99999999999999999.99" {
		t.Errorf("price = %s, want 99999999999999999.99", got.Price)
	}
	if got.Count.String() != "12345678901234567" {
		t.Errorf("count = %s, want 12345678901234567", got.Count)
	}
}

// =========================================================================
// Tests — Path & Query Parameters
// =========================================================================

func TestE2E_PathParams(t *testing.T) {
	baseURL := newTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/items/42/widget")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	var got itemResp
	if err := c.Do(req, http.StatusOK, client.WithDestination(&got)); err != nil {
		t.Fatalf("executing request: %v", err)
	}

	if got.ID != "42" {
		t.Errorf("id = %q, want %q", got.ID, "42")
	}
	if got.Name != "widget" {
		t.Errorf("name = %q, want %q", got.Name, "widget")
	}
}

func TestE2E_ParamInt(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/typed-item/256")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	var got typedItemResp
	if err := c.Do(req, http.StatusOK, client.WithDestination(&got)); err != nil {
		t.Fatalf("executing request: %v", err)
	}

	if got.ID != 256 {
		t.Errorf("id = %d, want 256", got.ID)
	}
	if got.Name != "typed" {
		t.Errorf("name = %q, want %q", got.Name, "typed")
	}
}

func TestE2E_ParamInt_Invalid(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/typed-item/abc")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Do(req, http.StatusOK)

	var statusErr *client.UnexpectedStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected UnexpectedStatusError, got %T: %v", err, err)
	}
	if statusErr.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", statusErr.StatusCode, http.StatusBadRequest)
	}
}

func TestE2E_QueryParams(t *testing.T) {
	baseURL := newTestApp(t)
	c := newClient(t)

	scheme, host := splitBaseURL(t, baseURL)

	reqURL := c.URL(scheme, host, "/query",
		client.WithQueryStrings(map[string]string{
			"search": "gopher",
			"page":   "3",
		}),
	)

	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	var got queryResp
	if err := c.Do(req, http.StatusOK, client.WithDestination(&got)); err != nil {
		t.Fatalf("executing request: %v", err)
	}

	if got.Search != "gopher" {
		t.Errorf("search = %q, want %q", got.Search, "gopher")
	}
	if got.Page != "3" {
		t.Errorf("page = %q, want %q", got.Page, "3")
	}
}

func TestE2E_TypedQueryParams(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	scheme, host := splitBaseURL(t, baseURL)

	reqURL := c.URL(scheme, host, "/typed-query",
		client.WithQueryStrings(map[string]string{
			"search":  "rust",
			"page":    "7",
			"enabled": "true",
		}),
	)

	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	var got typedQueryResp
	if err := c.Do(req, http.StatusOK, client.WithDestination(&got)); err != nil {
		t.Fatalf("executing request: %v", err)
	}

	if got.Search != "rust" {
		t.Errorf("search = %q, want %q", got.Search, "rust")
	}
	if got.Page != 7 {
		t.Errorf("page = %d, want 7", got.Page)
	}
	if !got.Enabled {
		t.Error("enabled = false, want true")
	}
}

// =========================================================================
// Tests — Client Options
// =========================================================================

func TestE2E_UserAgent(t *testing.T) {
	baseURL := newFullTestApp(t)

	c, err := client.Build(client.WithUserAgent("httper-test/1.0"))
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	reqURL := mustParseURL(t, baseURL, "/echo-headers")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	var got headerEcho
	if err := c.Do(req, http.StatusOK, client.WithDestination(&got)); err != nil {
		t.Fatalf("executing request: %v", err)
	}

	if got.UserAgent != "httper-test/1.0" {
		t.Errorf("user-agent = %q, want %q", got.UserAgent, "httper-test/1.0")
	}
}

func TestE2E_CustomHeaders(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/echo-headers")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet,
		client.WithHeaders(map[string][]string{
			"X-Custom-Header": {"custom-value"},
		}),
	)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	var got headerEcho
	if err := c.Do(req, http.StatusOK, client.WithDestination(&got)); err != nil {
		t.Fatalf("executing request: %v", err)
	}

	if got.Custom != "custom-value" {
		t.Errorf("custom header = %q, want %q", got.Custom, "custom-value")
	}
}

func TestE2E_ContentType(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/echo-headers")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet,
		client.WithContentType("application/xml"),
	)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	var got headerEcho
	if err := c.Do(req, http.StatusOK, client.WithDestination(&got)); err != nil {
		t.Fatalf("executing request: %v", err)
	}

	if got.ContentType != "application/xml" {
		t.Errorf("content-type = %q, want %q", got.ContentType, "application/xml")
	}
}

func TestE2E_Cookies(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/echo-cookies")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet,
		client.WithCookies(
			&http.Cookie{Name: "session", Value: "abc123"},
			&http.Cookie{Name: "token", Value: "xyz789"},
		),
	)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	var got cookieEcho
	if err := c.Do(req, http.StatusOK, client.WithDestination(&got)); err != nil {
		t.Fatalf("executing request: %v", err)
	}

	if got.Session != "abc123" {
		t.Errorf("session = %q, want %q", got.Session, "abc123")
	}
	if got.Token != "xyz789" {
		t.Errorf("token = %q, want %q", got.Token, "xyz789")
	}
}

func TestE2E_NoFollowRedirects(t *testing.T) {
	baseURL := newFullTestApp(t)

	c, err := client.Build(client.WithNoFollowRedirects())
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	reqURL := mustParseURL(t, baseURL, "/redirect")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	// With NoFollowRedirects, the 302 is the final response.
	// Client.Do expects 302, so it should succeed.
	if err := c.Do(req, http.StatusFound); err != nil {
		t.Fatalf("expected 302, got error: %v", err)
	}
}

func TestE2E_FollowRedirectsByDefault(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/redirect")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	// Default client follows redirects, landing on /redirect-target with 200.
	var got map[string]string
	if err := c.Do(req, http.StatusOK, client.WithDestination(&got)); err != nil {
		t.Fatalf("expected 200 after redirect, got error: %v", err)
	}

	if got["arrived"] != "true" {
		t.Errorf("arrived = %q, want %q", got["arrived"], "true")
	}
}

func TestE2E_Timeout(t *testing.T) {
	log := testLogger(t)

	app := mux.New(
		mux.WithMiddleware(
			middleware.Logger(log),
			middleware.Errors(log),
			middleware.Panics(),
		),
		mux.WithLogger(log),
	)

	app.Get("/slow", func(_ context.Context, _ http.ResponseWriter, _ *http.Request) error {
		time.Sleep(2 * time.Second)
		return nil
	})

	srv := httptest.NewServer(app)
	t.Cleanup(srv.Close)

	c, err := client.Build(client.WithTimeout(100 * time.Millisecond))
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	reqURL := mustParseURL(t, srv.URL, "/slow")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Do(req, http.StatusOK)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	if !strings.Contains(err.Error(), "Client.Timeout") && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected timeout-related error, got: %v", err)
	}
}

func TestE2E_Throttle(t *testing.T) {
	baseURL := newFullTestApp(t)

	// 2 RPS with burst of 2 — the first 2 requests should be near-instant,
	// the third should be delayed by rate limiting.
	c, err := client.Build(client.WithThrottle(2, 2))
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	reqURL := mustParseURL(t, baseURL, "/echo-headers")

	start := time.Now()
	for i := range 4 {
		req, err := c.Request(context.Background(), reqURL, http.MethodGet)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if err := c.Do(req, http.StatusOK); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)

	// With 2 RPS, 4 requests should take at least ~1 second (burst covers first 2).
	if elapsed < 500*time.Millisecond {
		t.Errorf("throttle didn't slow requests: 4 requests completed in %v", elapsed)
	}
}

func TestE2E_ContextCancellation(t *testing.T) {
	log := testLogger(t)

	app := mux.New(
		mux.WithMiddleware(
			middleware.Logger(log),
			middleware.Errors(log),
			middleware.Panics(),
		),
		mux.WithLogger(log),
	)

	app.Get("/hang", func(ctx context.Context, _ http.ResponseWriter, _ *http.Request) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})

	srv := httptest.NewServer(app)
	t.Cleanup(srv.Close)

	c := newClient(t)

	ctx, cancel := context.WithCancel(context.Background())

	reqURL := mustParseURL(t, srv.URL, "/hang")
	req, err := c.Request(ctx, reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	// Cancel after a short delay.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err = c.Do(req, http.StatusOK)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		// The error may be wrapped; just check the string as fallback.
		if !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	}
}

// =========================================================================
// Tests — Error Handling
// =========================================================================

func TestE2E_ErrorHandling(t *testing.T) {
	baseURL := newTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/error/not-found")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Do(req, http.StatusOK)

	var statusErr *client.UnexpectedStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected UnexpectedStatusError, got %T: %v", err, err)
	}

	if statusErr.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", statusErr.StatusCode, http.StatusNotFound)
	}

	wantBody := `{"code":404,"message":"widget not found"}`
	if statusErr.Body != wantBody {
		t.Errorf("body = %q, want %q", statusErr.Body, wantBody)
	}
}

func TestE2E_InternalErrorObscured(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/error/internal")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Do(req, http.StatusOK)

	var statusErr *client.UnexpectedStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected UnexpectedStatusError, got %T: %v", err, err)
	}

	if statusErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", statusErr.StatusCode, http.StatusInternalServerError)
	}

	// The internal error message should be obscured — the client must not
	// see "database connection failed" or "secret-dsn".
	if strings.Contains(statusErr.Body, "secret-dsn") {
		t.Error("internal error detail leaked to client")
	}
	if strings.Contains(statusErr.Body, "database connection failed") {
		t.Error("internal error message leaked to client")
	}
	if !strings.Contains(statusErr.Body, "Internal Server Error") {
		t.Errorf("expected obscured message, got: %s", statusErr.Body)
	}
}

func TestE2E_AuthFailure(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	tests := []struct {
		name string
		path string
		code int
	}{
		{"unauthorized", "/error/unauthorized", http.StatusUnauthorized},
		{"forbidden", "/error/forbidden", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqURL := mustParseURL(t, baseURL, tt.path)
			req, err := c.Request(context.Background(), reqURL, http.MethodGet)
			if err != nil {
				t.Fatalf("creating request: %v", err)
			}

			err = c.Do(req, http.StatusOK)

			var statusErr *client.UnexpectedStatusError
			if !errors.As(err, &statusErr) {
				t.Fatalf("expected UnexpectedStatusError, got %T: %v", err, err)
			}

			if statusErr.StatusCode != tt.code {
				t.Errorf("status = %d, want %d", statusErr.StatusCode, tt.code)
			}

			// Client should join ErrAuthFailure for 401/403.
			if !errors.Is(err, client.ErrAuthFailure) {
				t.Error("expected error to wrap ErrAuthFailure")
			}
			if !errors.Is(err, client.ErrUnexpectedStatusCode) {
				t.Error("expected error to wrap ErrUnexpectedStatusCode")
			}
		})
	}
}

func TestE2E_FieldValidationErrors(t *testing.T) {
	baseURL := newTestApp(t)
	c := newClient(t)

	// Send invalid payload: missing name, invalid email.
	payload := struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}{
		Name:  "",
		Email: "not-an-email",
	}

	reqURL := mustParseURL(t, baseURL, "/validate")
	req, err := c.Request(context.Background(), reqURL, http.MethodPost, client.WithPayload(payload))
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Do(req, http.StatusOK)

	var statusErr *client.UnexpectedStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected UnexpectedStatusError, got %T: %v", err, err)
	}

	if statusErr.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", statusErr.StatusCode, http.StatusUnprocessableEntity)
	}

	if statusErr.Body == "" {
		t.Fatal("expected non-empty body with field errors")
	}

	var fields []errs.FieldError
	if err := json.Unmarshal([]byte(statusErr.Body), &fields); err != nil {
		t.Fatalf("parsing field errors: %v\nbody: %s", err, statusErr.Body)
	}

	if len(fields) < 2 {
		t.Fatalf("expected at least 2 field errors, got %d: %v", len(fields), fields)
	}
}

func TestE2E_PanicRecovery(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/panic")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Do(req, http.StatusOK)

	var statusErr *client.UnexpectedStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected UnexpectedStatusError, got %T: %v", err, err)
	}

	// Panics middleware recovers → Errors middleware wraps → 500.
	if statusErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", statusErr.StatusCode, http.StatusInternalServerError)
	}
}

func TestE2E_DecodeRejectsUnknownFields(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	// "extra" field is not in the user struct.
	payload := map[string]any{
		"name":  "Alice",
		"email": "alice@test.com",
		"age":   30,
		"extra": "should-be-rejected",
	}

	reqURL := mustParseURL(t, baseURL, "/strict-decode")
	req, err := c.Request(context.Background(), reqURL, http.MethodPost, client.WithPayload(payload))
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Do(req, http.StatusOK)

	var statusErr *client.UnexpectedStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected UnexpectedStatusError, got %T: %v", err, err)
	}

	if statusErr.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", statusErr.StatusCode, http.StatusBadRequest)
	}
}

func TestE2E_DecodeAllowUnknownFields(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	payload := map[string]any{
		"name":  "Alice",
		"email": "alice@test.com",
		"age":   30,
		"extra": "should-be-ignored",
	}

	reqURL := mustParseURL(t, baseURL, "/loose-decode")
	req, err := c.Request(context.Background(), reqURL, http.MethodPost, client.WithPayload(payload))
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	var got user
	if err := c.Do(req, http.StatusOK, client.WithDestination(&got)); err != nil {
		t.Fatalf("expected 200, got error: %v", err)
	}

	if got.Name != "Alice" {
		t.Errorf("name = %q, want %q", got.Name, "Alice")
	}
}

// =========================================================================
// Tests — Download
// =========================================================================

func TestE2E_FileDownload(t *testing.T) {
	baseURL := newTestApp(t)
	c := newClient(t)

	destPath := filepath.Join(t.TempDir(), "downloaded.bin")

	reqURL := mustParseURL(t, baseURL, "/download")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	if err := c.Download(req, http.StatusOK, destPath); err != nil {
		t.Fatalf("downloading: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}

	want := "hello, this is test download content!"
	if string(got) != want {
		t.Errorf("file content = %q, want %q", string(got), want)
	}

	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != int64(len(want)) {
		t.Errorf("file size = %d, want %d", info.Size(), len(want))
	}
}

func TestE2E_DownloadChecksum(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	data := []byte("hello, this is test download content!")
	h := sha256.Sum256(data)
	expected := hex.EncodeToString(h[:])

	destPath := filepath.Join(t.TempDir(), "checksum-ok.bin")

	reqURL := mustParseURL(t, baseURL, "/download")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	if err := c.Download(req, http.StatusOK, destPath, download.WithChecksum(sha256.New(), expected)); err != nil {
		t.Fatalf("download with valid checksum failed: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content mismatch after checksum download")
	}
}

func TestE2E_DownloadChecksumMismatch(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	destPath := filepath.Join(t.TempDir(), "checksum-bad.bin")

	reqURL := mustParseURL(t, baseURL, "/download")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Download(req, http.StatusOK, destPath, download.WithChecksum(sha256.New(), "badhash"))
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}

	if !errors.Is(err, download.ErrChecksumMismatch) {
		t.Errorf("expected ErrChecksumMismatch, got: %v", err)
	}

	// File should not exist after failed checksum.
	if _, err := os.Stat(destPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected file to be cleaned up, but it exists")
	}
}

func TestE2E_DownloadSkipExisting(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	destPath := filepath.Join(t.TempDir(), "existing.bin")

	// Pre-create the file with different content.
	if err := os.WriteFile(destPath, []byte("pre-existing"), 0644); err != nil {
		t.Fatal(err)
	}

	reqURL := mustParseURL(t, baseURL, "/download")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	if err := c.Download(req, http.StatusOK, destPath, download.WithSkipExisting()); err != nil {
		t.Fatalf("download with skip existing failed: %v", err)
	}

	// File should still have original content (download skipped).
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pre-existing" {
		t.Errorf("file was overwritten: got %q, want %q", string(got), "pre-existing")
	}
}

// =========================================================================
// Tests — CORS Middleware
// =========================================================================

func TestE2E_MiddlewareCORS(t *testing.T) {
	baseURL := newTestApp(t)
	c := newClient(t)

	sent := user{Name: "Bob", Email: "bob@test.com", Age: 25}

	reqURL := mustParseURL(t, baseURL, "/echo")
	req, err := c.Request(context.Background(), reqURL, http.MethodPost, client.WithPayload(sent))
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	req.Header.Set("Origin", "https://allowed.example.com")

	resp, err := c.InternalClient().Do(req)
	if err != nil {
		t.Fatalf("executing raw request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	acao := resp.Header.Get("Access-Control-Allow-Origin")
	if acao != "https://allowed.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", acao, "https://allowed.example.com")
	}

	vary := resp.Header.Get("Vary")
	if vary != "Origin" {
		t.Errorf("Vary = %q, want %q", vary, "Origin")
	}

	acac := resp.Header.Get("Access-Control-Allow-Credentials")
	if acac != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want %q", acac, "true")
	}
}

func TestE2E_CORSDisallowedOrigin(t *testing.T) {
	baseURL := newTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/echo-headers")
	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://evil.example.com")

	resp, err := c.InternalClient().Do(req)
	if err != nil {
		t.Fatalf("executing request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}

	// Disallowed origin should not get CORS headers.
	if acao := resp.Header.Get("Access-Control-Allow-Origin"); acao != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", acao)
	}
}

func TestE2E_CORSPreflight(t *testing.T) {
	baseURL := newTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/echo")
	req, err := http.NewRequest(http.MethodOptions, reqURL.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://allowed.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	resp, err := c.InternalClient().Do(req)
	if err != nil {
		t.Fatalf("executing request: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	if acao := resp.Header.Get("Access-Control-Allow-Origin"); acao != "https://allowed.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", acao, "https://allowed.example.com")
	}

	acam := resp.Header.Get("Access-Control-Allow-Methods")
	if !strings.Contains(acam, "POST") {
		t.Errorf("Access-Control-Allow-Methods = %q, should contain POST", acam)
	}

	acma := resp.Header.Get("Access-Control-Max-Age")
	if acma != "86400" {
		t.Errorf("Access-Control-Max-Age = %q, want %q", acma, "86400")
	}
}

func TestE2E_CORSNoOriginHeader(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/echo-headers")
	req, err := c.Request(context.Background(), reqURL, http.MethodGet)
	if err != nil {
		t.Fatal(err)
	}
	// No Origin header set.

	resp, err := c.InternalClient().Do(req)
	if err != nil {
		t.Fatalf("executing request: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	// Without Origin, CORS middleware is a no-op — no CORS headers.
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if acao := resp.Header.Get("Access-Control-Allow-Origin"); acao != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty (no Origin sent)", acao)
	}
}

// =========================================================================
// Tests — Routing
// =========================================================================

func TestE2E_MountSubRoute(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	t.Run("get", func(t *testing.T) {
		reqURL := mustParseURL(t, baseURL, "/api/v1/status")
		req, err := c.Request(context.Background(), reqURL, http.MethodGet)
		if err != nil {
			t.Fatalf("creating request: %v", err)
		}

		var got map[string]string
		if err := c.Do(req, http.StatusOK, client.WithDestination(&got)); err != nil {
			t.Fatalf("executing request: %v", err)
		}

		if got["status"] != "ok" {
			t.Errorf("status = %q, want %q", got["status"], "ok")
		}
	})

	t.Run("post", func(t *testing.T) {
		sent := user{Name: "MountedUser", Email: "mount@test.com", Age: 1}

		reqURL := mustParseURL(t, baseURL, "/api/v1/items")
		req, err := c.Request(context.Background(), reqURL, http.MethodPost, client.WithPayload(sent))
		if err != nil {
			t.Fatalf("creating request: %v", err)
		}

		var got user
		if err := c.Do(req, http.StatusCreated, client.WithDestination(&got)); err != nil {
			t.Fatalf("executing request: %v", err)
		}

		if got != sent {
			t.Errorf("mount echo mismatch:\n  got:  %+v\n  want: %+v", got, sent)
		}
	})
}

func TestE2E_GroupMiddleware(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	t.Run("no_auth_rejected", func(t *testing.T) {
		reqURL := mustParseURL(t, baseURL, "/admin/dashboard")
		req, err := c.Request(context.Background(), reqURL, http.MethodGet)
		if err != nil {
			t.Fatalf("creating request: %v", err)
		}

		err = c.Do(req, http.StatusOK)

		var statusErr *client.UnexpectedStatusError
		if !errors.As(err, &statusErr) {
			t.Fatalf("expected UnexpectedStatusError, got %T: %v", err, err)
		}
		if statusErr.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", statusErr.StatusCode, http.StatusUnauthorized)
		}
	})

	t.Run("with_auth_accepted", func(t *testing.T) {
		reqURL := mustParseURL(t, baseURL, "/admin/dashboard")
		req, err := c.Request(context.Background(), reqURL, http.MethodGet,
			client.WithHeaders(map[string][]string{
				"X-Auth-Token": {"valid-token"},
			}),
		)
		if err != nil {
			t.Fatalf("creating request: %v", err)
		}

		var got map[string]string
		if err := c.Do(req, http.StatusOK, client.WithDestination(&got)); err != nil {
			t.Fatalf("expected 200, got error: %v", err)
		}

		if got["page"] != "dashboard" {
			t.Errorf("page = %q, want %q", got["page"], "dashboard")
		}
	})
}

func TestE2E_HandleRaw(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	reqURL := mustParseURL(t, baseURL, "/raw/health")
	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := c.InternalClient().Do(req)
	if err != nil {
		t.Fatalf("executing request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/plain" {
		t.Errorf("content-type = %q, want %q", ct, "text/plain")
	}
	if string(body) != "OK" {
		t.Errorf("body = %q, want %q", string(body), "OK")
	}
}

func TestE2E_StaticFS(t *testing.T) {
	baseURL := newFullTestApp(t)
	c := newClient(t)

	tests := []struct {
		name     string
		path     string
		contains string
	}{
		{"index", "/static/index.html", "<html><body>hello</body></html>"},
		{"css", "/static/assets/style.css", "body { color: red; }"},
		{"favicon", "/static/assets/favicon.ico", "icon-data"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqURL := mustParseURL(t, baseURL, tt.path)
			req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
			if err != nil {
				t.Fatal(err)
			}

			resp, err := c.InternalClient().Do(req)
			if err != nil {
				t.Fatalf("executing request: %v", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			if !strings.Contains(string(body), tt.contains) {
				t.Errorf("body = %q, want to contain %q", string(body), tt.contains)
			}
		})
	}
}
