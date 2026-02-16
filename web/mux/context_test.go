package mux_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/adamwoolhether/httper/web/mux"
)

func TestGetValues_NoValues(t *testing.T) {
	v := mux.GetValues(context.Background())

	if v.TraceID != uuid.Nil.String() {
		t.Fatalf("TraceID = %q, want %q", v.TraceID, uuid.Nil.String())
	}
	if v.Tracer == nil {
		t.Fatal("Tracer should be non-nil (noop)")
	}
	if v.Now.IsZero() {
		t.Fatal("Now should be non-zero")
	}
}

func TestGetTraceID_NoValues(t *testing.T) {
	id := mux.GetTraceID(context.Background())
	if id != uuid.Nil.String() {
		t.Fatalf("GetTraceID = %q, want %q", id, uuid.Nil.String())
	}
}

func TestSetStatusCode_NoValues(t *testing.T) {
	// Should not panic on bare context with no BaseValues.
	mux.SetStatusCode(context.Background(), 200)
}

func TestAddSpan_NoValues(t *testing.T) {
	ctx := context.Background()
	newCtx, span := mux.AddSpan(ctx, "test-span")

	if newCtx != ctx {
		t.Fatal("AddSpan should return original context when no BaseValues")
	}
	if span == nil {
		t.Fatal("span should not be nil")
	}
}
