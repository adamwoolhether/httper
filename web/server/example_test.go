package server_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/adamwoolhether/httper/web/server"
)

func ExampleNew() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello")
	})

	srv := server.New(mux,
		server.WithHost(":3000"),
		server.WithReadTimeout(10*time.Second),
		server.WithLogger(slog.Default()),
	)

	_ = srv // srv.Run() blocks until signal

	fmt.Println("server configured")
	// Output: server configured
}

func ExampleWithShutdownFunc() {
	cleanup := func(ctx context.Context) error {
		fmt.Println("closing database")
		return nil
	}

	mux := http.NewServeMux()
	srv := server.New(mux,
		server.WithShutdownFunc(cleanup),
	)

	// Simulate a graceful shutdown instead of calling srv.Run(),
	// which blocks on OS signals.
	if err := srv.Shutdown(context.Background()); err != nil {
		fmt.Println("shutdown error:", err)
		return
	}

	fmt.Println("shutdown complete")
	// Output:
	// closing database
	// shutdown complete
}

func ExampleWithTLS() {
	srv := server.New(http.NewServeMux(),
		server.WithHost(":8443"),
		server.WithTLS("cert.pem", "key.pem"),
	)

	_ = srv

	fmt.Println("TLS configured")
	// Output: TLS configured
}
