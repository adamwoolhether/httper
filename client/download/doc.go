// Package download streams HTTP response bodies to disk with optional
// checksum validation and progress reporting.
//
// # Single Download
//
// [Handle] writes the response body to a temporary file alongside the
// destination path, then atomically renames it on success:
//
//	err := download.Handle(ctx, resp.Body, resp.ContentLength, destPath, logger,
//		download.Options{},
//	)
//
// Most callers should use the higher-level
// [github.com/adamwoolhether/httper/client] package, which invokes
// Handle internally and re-exports all download options as
// client.With* functions.
package download
