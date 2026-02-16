// Package server manages the HTTP server lifecycle with graceful shutdown.
//
// It wraps [net/http.Server] and handles OS signal interception (SIGINT,
// SIGTERM), in-flight request draining, and ordered cleanup of external
// resources.
//
// Basic usage:
//
//	srv := server.New(mux, server.WithHost(":3000"))
//	if err := srv.Run(); err != nil {
//		log.Fatal(err)
//	}
//
// Registering shutdown hooks:
//
//	srv := server.New(mux,
//		server.WithShutdownFunc(func(ctx context.Context) error {
//			return db.Close()
//		}),
//	)
package server
