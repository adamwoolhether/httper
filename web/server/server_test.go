package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestNew_Defaults(t *testing.T) {
	srv := New(http.NewServeMux())

	if srv.srv.Addr != ":8080" {
		t.Errorf("addr = %q, want %q", srv.srv.Addr, ":8080")
	}
	if srv.srv.ReadTimeout != 5*time.Second {
		t.Errorf("read timeout = %v, want %v", srv.srv.ReadTimeout, 5*time.Second)
	}
	if srv.srv.WriteTimeout != 10*time.Second {
		t.Errorf("write timeout = %v, want %v", srv.srv.WriteTimeout, 10*time.Second)
	}
	if srv.srv.IdleTimeout != 120*time.Second {
		t.Errorf("idle timeout = %v, want %v", srv.srv.IdleTimeout, 120*time.Second)
	}
	if srv.shutdownTimeout != 20*time.Second {
		t.Errorf("shutdown timeout = %v, want %v", srv.shutdownTimeout, 20*time.Second)
	}
	if srv.logger == nil {
		t.Error("logger is nil, want slog.Default()")
	}
}

func TestNew_WithOptions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	fn := func(ctx context.Context) error { return nil }

	srv := New(http.NewServeMux(),
		WithHost(":9090"),
		WithReadTimeout(1*time.Second),
		WithWriteTimeout(2*time.Second),
		WithIdleTimeout(3*time.Second),
		WithShutdownTimeout(4*time.Second),
		WithLogger(logger),
		WithShutdownFunc(fn),
		WithTLS("cert.pem", "key.pem"),
	)

	if srv.srv.Addr != ":9090" {
		t.Errorf("addr = %q, want %q", srv.srv.Addr, ":9090")
	}
	if srv.srv.ReadTimeout != 1*time.Second {
		t.Errorf("read timeout = %v, want %v", srv.srv.ReadTimeout, 1*time.Second)
	}
	if srv.srv.WriteTimeout != 2*time.Second {
		t.Errorf("write timeout = %v, want %v", srv.srv.WriteTimeout, 2*time.Second)
	}
	if srv.srv.IdleTimeout != 3*time.Second {
		t.Errorf("idle timeout = %v, want %v", srv.srv.IdleTimeout, 3*time.Second)
	}
	if srv.shutdownTimeout != 4*time.Second {
		t.Errorf("shutdown timeout = %v, want %v", srv.shutdownTimeout, 4*time.Second)
	}
	if srv.logger != logger {
		t.Error("logger not set correctly")
	}
	if len(srv.shutdownFuncs) != 1 {
		t.Errorf("shutdown funcs = %d, want 1", len(srv.shutdownFuncs))
	}
	if srv.tlsCertFile != "cert.pem" {
		t.Errorf("tls cert = %q, want %q", srv.tlsCertFile, "cert.pem")
	}
	if srv.tlsKeyFile != "key.pem" {
		t.Errorf("tls key = %q, want %q", srv.tlsKeyFile, "key.pem")
	}
}

func TestRun_GracefulShutdown(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := New(mux, WithHost(":0"))

	// Find the actual port by starting a temporary listener.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	srv.srv.Addr = fmt.Sprintf(":%d", port)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run()
	}()

	// Wait for the server to be ready.
	addr := fmt.Sprintf("http://localhost:%d/health", port)
	waitForServer(t, addr, 2*time.Second)

	// Send SIGINT to trigger shutdown.
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return within 5s")
	}
}

func TestRun_ServerError(t *testing.T) {
	// Occupy a port so the server can't bind.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	srv := New(http.NewServeMux(), WithHost(fmt.Sprintf(":%d", port)))

	err = srv.Run()
	if err == nil {
		t.Fatal("Run() = nil, want error for occupied port")
	}
}

func TestShutdown_CallsShutdownFuncs(t *testing.T) {
	var order []int

	srv := New(http.NewServeMux(),
		WithHost(":0"),
		WithShutdownFunc(func(ctx context.Context) error {
			order = append(order, 1)
			return nil
		}),
		WithShutdownFunc(func(ctx context.Context) error {
			order = append(order, 2)
			return nil
		}),
		WithShutdownFunc(func(ctx context.Context) error {
			order = append(order, 3)
			return nil
		}),
	)

	// Start and then immediately shut down.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	srv.srv.Addr = fmt.Sprintf(":%d", port)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run()
	}()

	addr := fmt.Sprintf("http://localhost:%d/", port)
	waitForServer(t, addr, 2*time.Second)

	syscall.Kill(syscall.Getpid(), syscall.SIGINT)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return within 5s")
	}

	if len(order) != 3 {
		t.Fatalf("shutdown funcs called = %d, want 3", len(order))
	}
	for i, v := range order {
		if v != i+1 {
			t.Errorf("order[%d] = %d, want %d", i, v, i+1)
		}
	}
}

func TestShutdown_Timeout(t *testing.T) {
	var closed atomic.Bool

	mux := http.NewServeMux()
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := New(mux,
		WithHost(":0"),
		WithShutdownFunc(func(ctx context.Context) error {
			// Block until the caller's context expires.
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				closed.Store(true)
				return ctx.Err()
			}
			return nil
		}),
	)

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	srv.srv.Addr = fmt.Sprintf(":%d", port)

	go srv.srv.ListenAndServe()

	addr := fmt.Sprintf("http://localhost:%d/", port)
	waitForServer(t, addr, 2*time.Second)

	// Start a long-running request so Shutdown can't drain it.
	go http.Get(fmt.Sprintf("http://localhost:%d/slow", port))
	time.Sleep(50 * time.Millisecond)

	// Caller controls the deadline via context.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = srv.Shutdown(ctx)
	if err == nil {
		t.Fatal("Shutdown() = nil, want timeout error")
	}

	if !closed.Load() {
		t.Error("shutdown func context was not cancelled")
	}
}

func TestRun_TLS(t *testing.T) {
	certFile, keyFile := generateSelfSignedCert(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	srv := New(mux,
		WithHost(fmt.Sprintf(":%d", port)),
		WithTLS(certFile, keyFile),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run()
	}()

	// Wait for TLS server to be ready.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	addr := fmt.Sprintf("https://localhost:%d/health", port)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(addr)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}

	syscall.Kill(syscall.Getpid(), syscall.SIGINT)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return within 5s")
	}
}

// waitForServer polls the addr until it gets a response or the timeout expires.
func waitForServer(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(addr)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("server at %s not ready within %v", addr, timeout)
}

// generateSelfSignedCert creates a temporary self-signed certificate
// and returns paths to the cert and key PEM files.
func generateSelfSignedCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()

	certPath := filepath.Join(dir, "cert.pem")
	certOut, err := os.Create(certPath)
	if err != nil {
		t.Fatal(err)
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	certOut.Close()

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}

	keyPath := filepath.Join(dir, "key.pem")
	keyOut, err := os.Create(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	keyOut.Close()

	return certPath, keyPath
}
