//go:build integration

package client_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adamwoolhether/httper/client"
	"github.com/adamwoolhether/httper/client/download"
)

func TestIntegration_Download_RemoteSmallFile(t *testing.T) {
	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	u, err := url.Parse("https://go.dev/VERSION?m=text")
	if err != nil {
		t.Fatalf("parsing URL: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "VERSION")

	req, err := c.Request(t.Context(), u, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	if err := c.Download(req, http.StatusOK, destPath); err != nil {
		t.Fatalf("download failed: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}

	if len(got) == 0 {
		t.Fatal("downloaded file is empty")
	}

	if !strings.HasPrefix(string(got), "go") {
		t.Errorf("expected content to start with %q, got %q", "go", string(got))
	}
}

func TestIntegration_Download_RemoteWithChecksum(t *testing.T) {
	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	u, err := url.Parse("https://go.dev/VERSION?m=text")
	if err != nil {
		t.Fatalf("parsing URL: %v", err)
	}

	// First, download the file to compute its checksum.
	firstPath := filepath.Join(t.TempDir(), "VERSION-first")

	req, err := c.Request(t.Context(), u, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	if err := c.Download(req, http.StatusOK, firstPath); err != nil {
		t.Fatalf("first download failed: %v", err)
	}

	content, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("reading first download: %v", err)
	}

	hash := sha256.Sum256(content)
	expChecksum := hex.EncodeToString(hash[:])

	// Now download again with checksum verification.
	secondPath := filepath.Join(t.TempDir(), "VERSION-verified")

	req, err = c.Request(t.Context(), u, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	err = c.Download(req, http.StatusOK, secondPath, download.WithChecksum(sha256.New(), expChecksum))
	if err != nil {
		t.Fatalf("checksum-verified download failed: %v", err)
	}

	got, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("reading verified download: %v", err)
	}

	if !bytes.Equal(got, content) {
		t.Error("verified download content differs from first download")
	}
}

func TestIntegration_Download_RemoteWithProgress(t *testing.T) {
	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	u, err := url.Parse("https://go.dev/VERSION?m=text")
	if err != nil {
		t.Fatalf("parsing URL: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "VERSION-progress")

	req, err := c.Request(t.Context(), u, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	if err := c.Download(req, http.StatusOK, destPath, download.WithProgress()); err != nil {
		t.Fatalf("download with progress failed: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}

	if len(got) == 0 {
		t.Fatal("downloaded file is empty")
	}

	if !strings.HasPrefix(string(got), "go") {
		t.Errorf("expected content to start with %q, got %q", "go", string(got))
	}
}

func TestIntegration_Download_RemoteSkipExisting(t *testing.T) {
	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	u, err := url.Parse("https://go.dev/VERSION?m=text")
	if err != nil {
		t.Fatalf("parsing URL: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "VERSION-existing")

	originalContent := []byte("already here")
	if err := os.WriteFile(destPath, originalContent, 0o644); err != nil {
		t.Fatalf("writing pre-existing file: %v", err)
	}

	req, err := c.Request(t.Context(), u, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	if err := c.Download(req, http.StatusOK, destPath, download.WithSkipExisting()); err != nil {
		t.Fatalf("download with skip existing failed: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	if !bytes.Equal(got, originalContent) {
		t.Errorf("file was overwritten; got %q, want %q", got, originalContent)
	}
}

func TestIntegration_DownloadAsync_RemoteSingle(t *testing.T) {
	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	u, err := url.Parse("https://go.dev/VERSION?m=text")
	if err != nil {
		t.Fatalf("parsing URL: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "VERSION-async")

	req, err := c.Request(t.Context(), u, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	r, err := c.DownloadAsync(req, http.StatusOK, destPath)
	if err != nil {
		t.Fatalf("starting async download: %v", err)
	}

	if err := r.Wait(); err != nil {
		t.Fatalf("async download failed: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}

	if len(got) == 0 {
		t.Fatal("downloaded file is empty")
	}

	if !strings.HasPrefix(string(got), "go") {
		t.Errorf("expected content to start with %q, got %q", "go", string(got))
	}
}

func TestIntegration_DownloadAsync_RemoteBatch(t *testing.T) {
	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	urls := []string{
		"https://go.dev/VERSION?m=text",
		"https://dl.google.com/go/go1.24.0.linux-amd64.tar.gz.sha256",
		"https://dl.google.com/go/go1.23.0.linux-amd64.tar.gz.sha256",
	}

	tmpDir := t.TempDir()

	u0, err := url.Parse(urls[0])
	if err != nil {
		t.Fatalf("parsing URL 0: %v", err)
	}

	req0, err := c.Request(t.Context(), u0, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request 0: %v", err)
	}

	r, err := c.DownloadAsync(req0, http.StatusOK, filepath.Join(tmpDir, "batch-0"), download.WithBatch(2))
	if err != nil {
		t.Fatalf("starting async download 0: %v", err)
	}

	for i := 1; i < len(urls); i++ {
		u, err := url.Parse(urls[i])
		if err != nil {
			t.Fatalf("parsing URL %d: %v", i, err)
		}

		req, err := c.Request(t.Context(), u, http.MethodGet)
		if err != nil {
			t.Fatalf("creating request %d: %v", i, err)
		}

		r.Add(req, http.StatusOK, filepath.Join(tmpDir, fmt.Sprintf("batch-%d", i)))
	}

	if err := r.Wait(); err != nil {
		t.Fatalf("batch download failed: %v", err)
	}

	// Verify VERSION file.
	got0, err := os.ReadFile(filepath.Join(tmpDir, "batch-0"))
	if err != nil {
		t.Fatalf("reading batch-0: %v", err)
	}
	if !strings.HasPrefix(string(got0), "go") {
		t.Errorf("batch-0: expected content to start with %q, got %q", "go", string(got0))
	}

	// Verify SHA256 checksum files are 64 hex chars (+ optional newline).
	for i := 1; i < len(urls); i++ {
		got, err := os.ReadFile(filepath.Join(tmpDir, fmt.Sprintf("batch-%d", i)))
		if err != nil {
			t.Fatalf("reading batch-%d: %v", i, err)
		}

		trimmed := strings.TrimSpace(string(got))
		if len(trimmed) != 64 {
			t.Errorf("batch-%d: expected 64 hex chars, got %d: %q", i, len(trimmed), trimmed)
		}
	}
}

func TestIntegration_Download_RemoteCancelMidDownload(t *testing.T) {
	c, err := client.Build()
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	// Use a larger file to ensure we can cancel mid-stream.
	// The Go source tarball is ~30MB; enough to allow cancellation.
	u, err := url.Parse("https://dl.google.com/go/go1.24.0.src.tar.gz")
	if err != nil {
		t.Fatalf("parsing URL: %v", err)
	}

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "cancel-me.tar.gz")

	ctx, cancel := context.WithCancel(t.Context())

	req, err := c.Request(ctx, u, http.MethodGet)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.Download(req, http.StatusOK, destPath)
	}()

	// Allow time for the download to start receiving data, then cancel.
	time.Sleep(500 * time.Millisecond)
	cancel()

	err = <-errCh
	if err == nil {
		t.Fatal("expected error after cancellation, got nil")
	}

	if !errors.Is(err, download.ErrDownloadCancelled) && !errors.Is(err, context.Canceled) {
		t.Errorf("expected ErrDownloadCancelled or context.Canceled, got: %v", err)
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
