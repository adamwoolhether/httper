package download

import (
	"errors"
	"hash"
)

// Option is a functional option for configuring a download via [Handle].
type Option func(*Options) error

// Options holds the resolved configuration for a single download.
type Options struct {
	checksum     *checksumVerifier
	progress     bool
	skipExisting bool
	Group        *Queue
}

// WithBatch activates batch mode by creating a Queue with the given
// concurrency limit. If maxConcurrent <= 0, concurrency is unlimited.
func WithBatch(maxConcurrent int) Option {
	return func(opts *Options) error {
		if opts.Group != nil {
			return errors.New("WithBatch cannot be used with Result.Add")
		}
		opts.Group = newQueue(maxConcurrent)
		return nil
	}
}

func withBatch(queue *Queue) Option {
	return func(opts *Options) error {
		opts.Group = queue
		return nil
	}
}

// WithChecksum enables checksum validation of the downloaded file.
// h is a [hash.Hash] instance (e.g. sha256.New()), and expected is the
// hex-encoded expected checksum string.
func WithChecksum(h hash.Hash, expected string) Option {
	return func(opts *Options) error {
		if h == nil {
			return errors.New("hash must not be nil")
		}

		if expected == "" {
			return errors.New("expected checksum must not be empty")
		}

		opts.checksum = &checksumVerifier{hash: h, expected: expected}
		return nil
	}
}

// WithProgress enables periodic download progress logging via the
// logger supplied to [Handle].
func WithProgress() Option {
	return func(opts *Options) error {
		opts.progress = true
		return nil
	}
}

// WithSkipExisting causes [Handle] to return nil immediately when
// the destination file already exists, avoiding a redundant download.
func WithSkipExisting() Option {
	return func(opts *Options) error {
		opts.skipExisting = true
		return nil
	}
}
