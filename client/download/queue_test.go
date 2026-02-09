package download

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestResult_Err(t *testing.T) {
	wantErr := errors.New("boom")
	g := newQueue(0)

	r := g.Start(t.Context(), func(ctx context.Context) error {
		return wantErr
	}, nil)

	if err := r.Err(); !errors.Is(err, wantErr) {
		t.Errorf("expected %v, got %v", wantErr, err)
	}
}

func TestResult_Err_Success(t *testing.T) {
	g := newQueue(0)

	r := g.Start(t.Context(), func(ctx context.Context) error {
		return nil
	}, nil)

	if err := r.Err(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestResult_Wait_SingleError(t *testing.T) {
	wantErr := errors.New("single fail")
	g := newQueue(0)

	r := g.Start(t.Context(), func(ctx context.Context) error {
		return wantErr
	}, nil)

	if err := r.Wait(); !errors.Is(err, wantErr) {
		t.Errorf("expected %v, got %v", wantErr, err)
	}
}

func TestResult_Wait_Success(t *testing.T) {
	g := newQueue(0)

	r := g.Start(t.Context(), func(ctx context.Context) error {
		return nil
	}, nil)

	if err := r.Wait(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestResult_Done(t *testing.T) {
	g := newQueue(0)

	r := g.Start(t.Context(), func(ctx context.Context) error {
		return nil
	}, nil)

	select {
	case <-r.Done():
	case <-time.After(time.Second):
		t.Fatal("Done channel was not closed in time")
	}
}

func TestGroup_Wait_JoinedErrors(t *testing.T) {
	err1 := errors.New("error one")
	err2 := errors.New("error two")
	g := newQueue(0)

	g.Start(t.Context(), func(ctx context.Context) error { return err1 }, nil)
	g.Start(t.Context(), func(ctx context.Context) error { return err2 }, nil)

	err := g.Wait()
	if err == nil {
		t.Fatal("expected joined error, got nil")
	}
	if !errors.Is(err, err1) {
		t.Errorf("expected error to contain %v", err1)
	}
	if !errors.Is(err, err2) {
		t.Errorf("expected error to contain %v", err2)
	}
}

func TestGroup_Wait_MixedSuccessAndError(t *testing.T) {
	wantErr := errors.New("only failure")
	g := newQueue(0)

	g.Start(t.Context(), func(ctx context.Context) error { return nil }, nil)
	g.Start(t.Context(), func(ctx context.Context) error { return wantErr }, nil)
	g.Start(t.Context(), func(ctx context.Context) error { return nil }, nil)

	err := g.Wait()
	if !errors.Is(err, wantErr) {
		t.Errorf("expected %v, got %v", wantErr, err)
	}
}

func TestGroup_ConcurrencyLimit(t *testing.T) {
	const limit = 2
	const total = 5

	g := newQueue(limit)

	var running atomic.Int32
	var maxRunning atomic.Int32
	barrier := make(chan struct{})

	for range total {
		g.Start(t.Context(), func(ctx context.Context) error {
			cur := running.Add(1)
			for {
				old := maxRunning.Load()
				if cur <= old || maxRunning.CompareAndSwap(old, cur) {
					break
				}
			}
			<-barrier
			running.Add(-1)
			return nil
		}, nil)
	}

	// Let all goroutines proceed concurrently.
	time.Sleep(50 * time.Millisecond)
	close(barrier)

	if err := g.Wait(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if peak := maxRunning.Load(); peak > limit {
		t.Errorf("max concurrent was %d, want <= %d", peak, limit)
	}
}

func TestGroup_UnlimitedConcurrency(t *testing.T) {
	const total = 10

	g := newQueue(0)

	var running atomic.Int32
	var maxRunning atomic.Int32
	barrier := make(chan struct{})

	for range total {
		g.Start(t.Context(), func(ctx context.Context) error {
			cur := running.Add(1)
			for {
				old := maxRunning.Load()
				if cur <= old || maxRunning.CompareAndSwap(old, cur) {
					break
				}
			}
			<-barrier
			running.Add(-1)
			return nil
		}, nil)
	}

	time.Sleep(50 * time.Millisecond)
	close(barrier)

	if err := g.Wait(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if peak := maxRunning.Load(); peak < int32(total) {
		t.Errorf("expected all %d to run concurrently, peak was %d", total, peak)
	}
}

func TestResult_Cancel(t *testing.T) {
	g := newQueue(0)

	started := make(chan struct{})

	r := g.Start(t.Context(), func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}, nil)

	<-started
	r.Cancel()

	err := r.Err()
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestGroup_ContextCancellationOnSemaphore(t *testing.T) {
	// Queue with limit 1, start a long-running task to fill the slot,
	// then start a second with a cancelled context. It should fail with
	// context.Canceled without blocking forever.
	g := newQueue(1)

	release := make(chan struct{})
	g.Start(t.Context(), func(ctx context.Context) error {
		<-release
		return nil
	}, nil)

	// Give goroutine time to acquire the semaphore.
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel before starting.

	r := g.Start(ctx, func(ctx context.Context) error {
		t.Error("work function should not have run")
		return nil
	}, nil)

	err := r.Err()
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	close(release)

	if err := g.Wait(); err == nil {
		t.Error("expected group error from cancelled task")
	}
}

func TestGroup_Shutdown(t *testing.T) {
	g := newQueue(1)

	// Fill the semaphore slot with a task that blocks on a channel.
	release := make(chan struct{})
	g.Start(t.Context(), func(ctx context.Context) error {
		<-release
		return nil
	}, nil)

	// Give goroutine time to acquire the slot.
	time.Sleep(20 * time.Millisecond)

	g.Shutdown()

	// Release the first task so the second can acquire the semaphore.
	close(release)

	r := g.Start(t.Context(), func(ctx context.Context) error {
		t.Error("work function should not have run after shutdown")
		return nil
	}, nil)

	err := r.Err()
	if !errors.Is(err, ErrGroupShutdown) {
		t.Errorf("expected ErrGroupShutdown, got %v", err)
	}
}

func TestGroup_Wait_NilWhenAllSucceed(t *testing.T) {
	g := newQueue(0)

	g.Start(t.Context(), func(ctx context.Context) error { return nil }, nil)
	g.Start(t.Context(), func(ctx context.Context) error { return nil }, nil)

	if err := g.Wait(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}
