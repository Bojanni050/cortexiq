package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPingWithRetrySucceedsAfterFailures(t *testing.T) {
	calls := 0
	ping := func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("not ready")
		}
		return nil
	}
	if err := pingWithRetry(context.Background(), ping, time.Second, time.Millisecond, 2*time.Millisecond); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
}

func TestPingWithRetryGivesUpAfterWindow(t *testing.T) {
	want := errors.New("still down")
	calls := 0
	ping := func(context.Context) error { calls++; return want }
	err := pingWithRetry(context.Background(), ping, 30*time.Millisecond, time.Millisecond, 5*time.Millisecond)
	if !errors.Is(err, want) {
		t.Fatalf("expected last ping error, got %v", err)
	}
	if calls < 2 {
		t.Fatalf("expected several attempts, got %d", calls)
	}
}

func TestPingWithRetryStopsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	ping := func(context.Context) error {
		calls++
		cancel()
		return errors.New("down")
	}
	if err := pingWithRetry(ctx, ping, time.Minute, time.Millisecond, time.Millisecond); err == nil {
		t.Fatal("expected an error after cancellation")
	}
	if calls != 1 {
		t.Fatalf("expected 1 attempt after cancel, got %d", calls)
	}
}
