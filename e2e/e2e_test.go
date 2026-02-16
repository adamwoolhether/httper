//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/adamwoolhether/httper/client"
	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/errs"
	"github.com/adamwoolhether/httper/web/middleware"
	"github.com/adamwoolhether/httper/web/mux"
)

// -------------------------------------------------------------------------
// Types
// -------------------------------------------------------------------------

type user struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

type itemResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type queryResp struct {
	Search string `json:"search"`
	Page   string `json:"page"`
}

type validateReq struct {
	Name  string `json:"name"  validate:"required"`
	Email string `json:"email" validate:"required,email"`
}

// -------------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------------

func newTestApp(t *testing.T) string {
	t.Helper()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

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

func registerRoutes(app *mux.App) {
	app.Post("/echo", echoHandler)
	app.Get("/items/{id}/{name}", itemHandler)
	app.Get("/query", queryHandler)
	app.Get("/error/not-found", notFoundHandler)
	app.Post("/validate", validateHandler)
	app.Get("/download", downloadHandler)
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

// -------------------------------------------------------------------------
// Handlers
// -------------------------------------------------------------------------

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

// -------------------------------------------------------------------------
// Tests
// -------------------------------------------------------------------------

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

func TestE2E_QueryParams(t *testing.T) {
	baseURL := newTestApp(t)
	c := newClient(t)

	base, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parsing base URL: %v", err)
	}

	reqURL := c.URL(base.Scheme, base.Host, "/query",
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
