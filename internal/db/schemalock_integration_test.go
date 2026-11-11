package db

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testDSN skips the test without a database, like the integration tests in brain.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("CORTEXIQ_TEST_DSN")
	if dsn == "" {
		t.Skip("CORTEXIQ_TEST_DSN not set; skipping integration test")
	}
	return dsn
}

// freshDatabase creates an empty database next to the test one and returns its
// DSN. Schema setup runs against an empty database here, which is the state in
// which concurrent starts collide hardest, and it must not touch the shared one.
func freshDatabase(t *testing.T, dsn string) string {
	t.Helper()
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })

	name := fmt.Sprintf("cortexiq_lock_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

// The MCP server and the dashboard start together and both run the schema setup.
// Without the lock the loser fails on "relation ... already exists" and has to be
// restarted by its supervisor; with it every start succeeds.
func TestIntegrationConcurrentOpenAllSucceed(t *testing.T) {
	dsn := freshDatabase(t, testDSN(t))

	const starts = 6
	var wg sync.WaitGroup
	errs := make([]error, starts)
	pools := make([]*pgxpool.Pool, starts)
	for i := 0; i < starts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pools[i], errs[i] = Open(context.Background(), dsn, "fake", 4)
		}(i)
	}
	wg.Wait()

	for i, p := range pools {
		if p != nil {
			defer p.Close()
		}
		if errs[i] != nil {
			t.Errorf("start %d failed: %v", i, errs[i])
		}
	}
}

func TestIntegrationSchemaLockSerialises(t *testing.T) {
	dsn := testDSN(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var inside, overlaps int32
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := withSchemaLock(context.Background(), pool, func() error {
				if atomic.AddInt32(&inside, 1) > 1 {
					atomic.AddInt32(&overlaps, 1)
				}
				time.Sleep(150 * time.Millisecond)
				atomic.AddInt32(&inside, -1)
				return nil
			})
			if err != nil {
				t.Errorf("withSchemaLock: %v", err)
			}
		}()
	}
	wg.Wait()

	if n := atomic.LoadInt32(&overlaps); n != 0 {
		t.Errorf("%d holders ran at the same time, want strictly one at a time", n)
	}
}

// A start-up that is cancelled while it waits for the lock must give up cleanly,
// must not steal the lock from the holder, and must not leave one behind.
func TestIntegrationSchemaLockWaitIsCancellable(t *testing.T) {
	dsn := testDSN(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	holding := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- withSchemaLock(context.Background(), pool, func() error {
			close(holding)
			<-release
			return nil
		})
	}()
	<-holding

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	ran := false
	err = withSchemaLock(ctx, pool, func() error { ran = true; return nil })
	if err == nil {
		t.Fatal("a waiter whose context expired must get an error")
	}
	if ran {
		t.Error("the waiter's work ran although the lock was held by someone else")
	}
	if time.Since(started) > 5*time.Second {
		t.Errorf("the cancelled wait took %s", time.Since(started))
	}

	close(release)
	if err := <-holderDone; err != nil {
		t.Fatalf("holder: %v", err)
	}

	// Nothing is left behind: the next caller gets the lock at once.
	next, cancelNext := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelNext()
	if err := withSchemaLock(next, pool, func() error { return nil }); err != nil {
		t.Errorf("lock was not released after the holder and the cancelled waiter: %v", err)
	}
}
