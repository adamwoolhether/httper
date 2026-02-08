// Package download enables streaming HTTP response bodies to disk
// with optional checksum validation and progress reporting.
package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Handle streams body to a temp file in the same as destPath and
// then renamed on success. On any error the temp file is removed.
func Handle(ctx context.Context, body io.Reader, contentLength int64, destPath string, logger *slog.Logger, optFns ...Option) error {
	var opts options
	for _, opt := range optFns {
		if err := opt(&opts); err != nil {
			return fmt.Errorf("applying option: %w", err)
		}
	}

	if opts.skipExisting {
		if _, err := os.Stat(destPath); err == nil {
			logger.Info("skipping existing file", "path", destPath)
			return nil
		}
	}

	body = &contextReader{ctx: ctx, r: body}

	file, err := os.CreateTemp(filepath.Dir(destPath), ".httper-dl-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	var successful bool
	defer func() {
		if err := file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			logger.Error("defer closing temp file", "error", err)
		}
		if !successful {
			if err := os.Remove(file.Name()); err != nil {
				logger.Error("failed to remove temp file", "error", err)
			}
		}
	}()

	var writer io.Writer = file
	if opts.checksum != nil {
		writer = io.MultiWriter(writer, opts.checksum)
	}

	if opts.progress {
		writer = &progressWriter{
			w:         writer,
			logger:    logger,
			total:     contentLength,
			startTime: time.Now(),
		}
	}

	n, err := io.Copy(writer, body)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("%w: %w", ErrDownloadCancelled, err)
		}

		return fmt.Errorf("copying file body: %w", err)
	}

	if contentLength >= 0 && n != contentLength {
		return &Error{
			Err:    ErrContentLengthMismatch,
			Detail: fmt.Sprintf("expected %d bytes, got %d", contentLength, n),
		}
	}

	if err := opts.checksum.Verify(); err != nil {
		return err
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(file.Name(), destPath); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}

	successful = true

	return nil
}
