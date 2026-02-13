package mux

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

type ctxKey int

const (
	base ctxKey = iota + 1
)

// BaseValues represents values that are shared across all requests for logging.
type BaseValues struct {
	TraceID    string
	Now        time.Time
	Tracer     trace.Tracer
	StatusCode int
}

// SetStatusCode updates the BaseValue's status code.
func SetStatusCode(ctx context.Context, statusCode int) {
	v, ok := ctx.Value(base).(*BaseValues)
	if !ok {
		return
	}

	v.StatusCode = statusCode
}

// GetValues retrieves the BaseValues from the given context.
func GetValues(ctx context.Context) *BaseValues {
	v, ok := ctx.Value(base).(*BaseValues)
	if !ok {
		return &BaseValues{
			TraceID: uuid.Nil.String(),
			Tracer:  noop.NewTracerProvider().Tracer(""),
			Now:     time.Now(),
		}
	}

	return v
}

// GetTraceID retrieves the current trace ID from the BaseValue in the given context.
// We return an empty uuid for testing purposes if not set.
func GetTraceID(ctx context.Context) string {
	v, ok := ctx.Value(base).(*BaseValues)
	if !ok {
		return uuid.Nil.String()
	}

	return v.TraceID
}

// AddSpan adds a span to the tracer, returning it and the context.
func AddSpan(ctx context.Context, spanName string, keyValues ...attribute.KeyValue) (context.Context, trace.Span) {
	v, ok := ctx.Value(base).(*BaseValues)
	if !ok || v.Tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}

	ctx, span := v.Tracer.Start(ctx, spanName)
	span.SetAttributes(keyValues...)

	return ctx, span
}

// setValues sets the specified BaseValues in the context.
func setValues(ctx context.Context, v *BaseValues) context.Context {
	return context.WithValue(ctx, base, v)
}
