package client

import (
	"hash"

	"github.com/adamwoolhether/httper/client/download"
)

// ————————————————————————————————————————————————————————————————————
// Type aliases – re-export user-facing types from [download].
// ————————————————————————————————————————————————————————————————————

type (
	// DownloadError wraps a sentinel error with additional detail.
	DownloadError = download.Error

	// DownloadResult represents an in-flight or completed async download.
	DownloadResult = download.Result
)

// ————————————————————————————————————————————————————————————————————
// Sentinel errors
// ————————————————————————————————————————————————————————————————————

var (
	// ErrContentLengthMismatch indicates the byte count did not match Content-Length.
	ErrContentLengthMismatch = download.ErrContentLengthMismatch

	// ErrChecksumMismatch indicates the file checksum did not match the expected value.
	ErrChecksumMismatch = download.ErrChecksumMismatch

	// ErrDownloadCancelled indicates the download was cancelled via context.
	ErrDownloadCancelled = download.ErrDownloadCancelled

	// ErrGroupShutdown indicates the download queue was shut down.
	ErrGroupShutdown = download.ErrGroupShutdown
)

// ————————————————————————————————————————————————————————————————————
// Download option forwarding functions
// ————————————————————————————————————————————————————————————————————

// WithChecksum enables checksum validation of the downloaded file.
// h is a [hash.Hash] instance (e.g. sha256.New()), and expected is the
// hex-encoded expected checksum string.
func WithChecksum(h hash.Hash, expected string) DownloadOption {
	return download.WithChecksum(h, expected)
}

// WithProgress enables periodic download progress logging.
func WithProgress() DownloadOption { return download.WithProgress() }

// WithSkipExisting causes a download to return nil immediately when
// the destination file already exists.
func WithSkipExisting() DownloadOption { return download.WithSkipExisting() }

// WithBatch activates batch mode by creating a download queue with the given
// concurrency limit. If maxConcurrent <= 0, concurrency is unlimited.
func WithBatch(maxConcurrent int) DownloadOption { return download.WithBatch(maxConcurrent) }
