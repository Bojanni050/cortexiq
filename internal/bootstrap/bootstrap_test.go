package bootstrap

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

// warmEmbedder is a Fake embedder that reports how often it was called and can
// hold the first call open so a test can control the ordering.
type warmEmbedder struct {
	calls   atomic.Int32
	started chan struct{}
	block   chan struct{}
	err     error
}

func newWarmEmbedder() *warmEmbedder {
	return &warmEmbedder{started: make(chan struct{})}
}

func (w *warmEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if w.calls.Add(1) == 1 {
		close(w.started)
	}
	if w.block != nil {
		select {
		case <-w.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if w.err != nil {
		return nil, w.err
	}
	return []float32{1, 0}, nil
}

func (w *warmEmbedder) Model() string { return "test" }
func (w *warmEmbedder) Dims() int     { return 2 }

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The warm-up must happen in the background (so start-up is not blocked) and
// exactly once per process, however many servers call it.
func TestWarmEmbedderRunsOnceInBackground(t *testing.T) {
	e := newWarmEmbedder()
	e.block = make(chan struct{})
	bc := &Context{Logger: discardLogger(), embedder: e}

	// A synchronous implementation would hang here on the blocked embedder, so
	// require the call to return before the block is released.
	returned := make(chan struct{})
	go func() {
		bc.WarmEmbedder(context.Background())
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("WarmEmbedder waited for the embedder instead of warming in the background")
	}
	// The first call is now in flight; a second call must not start another.
	<-e.started
	bc.WarmEmbedder(context.Background())
	close(e.block)

	waitFor(t, "the warm-up to finish", func() bool { return e.calls.Load() == 1 })
	time.Sleep(50 * time.Millisecond)
	if calls := e.calls.Load(); calls != 1 {
		t.Fatalf("embed calls = %d, want 1", calls)
	}
}

// A backend that refuses the warm-up must not break anything: recall reports
// that itself, with a better context.
func TestWarmEmbedderFailureIsHarmless(t *testing.T) {
	e := newWarmEmbedder()
	e.err = errors.New("backend down")
	bc := &Context{Logger: discardLogger(), embedder: e}

	bc.WarmEmbedder(context.Background())
	waitFor(t, "the warm-up to fail", func() bool { return e.calls.Load() == 1 })
}

// A Context without an embedder (a plain CLI bootstrap) is a no-op, not a crash.
func TestWarmEmbedderWithoutEmbedder(t *testing.T) {
	var bc *Context
	bc.WarmEmbedder(context.Background())
	(&Context{Logger: discardLogger()}).WarmEmbedder(context.Background())
}
