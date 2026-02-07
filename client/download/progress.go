package download

import (
	"fmt"
	"io"
	"log/slog"
	"time"
)

// progressWriter is an io.Writer, logging download progress at
// most once per second if enabled.
type progressWriter struct {
	w           io.Writer
	logger      *slog.Logger
	transferred int64
	total       int64
	startTime   time.Time
	lastLog     time.Time
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.w.Write(p)
	pw.transferred += int64(n)

	if time.Since(pw.lastLog) >= time.Second {
		pw.lastLog = time.Now()
		pw.log("downloading")
	}

	if pw.total >= 0 && pw.transferred == pw.total {
		pw.log("download complete")
	}

	return n, err
}

func (pw *progressWriter) log(msg string) {
	elapsed := time.Since(pw.startTime)
	attrs := []any{
		"progress", fmt.Sprintf("%.1f%%", float64(pw.transferred)/float64(pw.total)*100),
		"elapsed", elapsed.Round(time.Millisecond),
		"transferred", pw.transferred,
		"total", pw.total,
		"mbps", fmt.Sprintf("%.2f", float64(pw.transferred)/elapsed.Seconds()/(1024*1024)),
	}
	pw.logger.Info(msg, attrs...)
}
