package download

import (
	"errors"
	"hash"
)

// Option defines optional settings for downloading files.
// WithChecksum enables checksum validation of the downloaded file.
// h is a hash.Hash instance (e.g. sha256.New()), and expected is the
// hex-encoded expected checksum string.
//
// WithProgress enables periodic download progress logging via the
// logger supplied to Handle.
//
// WithSkipExisting causes Handle to return nil immediately when
// the destination file already exists, avoiding a redundant download.
type Option func(*options) error

type options struct {
	checksum     *checksumVerifier
	progress     bool
	skipExisting bool
}

func WithChecksum(h hash.Hash, expected string) Option {
	return func(opts *options) error {
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

func WithProgress() Option {
	return func(opts *options) error {
		opts.progress = true
		return nil
	}
}

func WithSkipExisting() Option {
	return func(opts *options) error {
		opts.skipExisting = true
		return nil
	}
}
