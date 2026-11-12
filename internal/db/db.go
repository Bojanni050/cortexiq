// Modifications copyright (c) 2026 Bojan van der Heide
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

type discardLogger struct{}

func (discardLogger) Printf(string, ...any) {}
func (discardLogger) Fatalf(string, ...any) {}

//go:embed migrations/*.sql
var embedMigrations embed.FS

// Open creates a pgxpool, runs goose migrations, and validates the embedding model setting.
// Returns the pool for application use. The caller is responsible for calling pool.Close().
func Open(ctx context.Context, dsn string, expectedModel string, vectorDim int) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.ParseConfig: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.NewWithConfig: %w", err)
	}

	// Retry only the connection itself: a database that is still starting (or
	// briefly unreachable) should not make the container exit and fall into
	// Docker's growing restart backoff. Migration and config errors below still
	// fail fast.
	if err := pingWithRetry(ctx, pool.Ping, connectRetryWindow, connectRetryInitial, connectRetryMax); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgxpool.Ping: %w", err)
	}

	// Open a *sql.DB backed by pgx for goose migrations.
	connConfig := pool.Config().ConnConfig
	sqlDB := stdlib.OpenDB(*connConfig)
	defer sqlDB.Close()

	// The MCP server and the dashboard are separate processes on the same
	// database and start together. Everything below is check-then-create, so two
	// of them running it at once collide ("relation already exists"); the loser
	// then crashes and only the restart policy brings it back. One at a time, and
	// the second finds nothing left to do.
	if err := withSchemaLock(ctx, pool, func() error {
		return setupSchema(ctx, pool, sqlDB, vectorDim)
	}); err != nil {
		pool.Close()
		return nil, err
	}

	// Validate dimension lock: vector dimension must match expected or be empty.
	// This allows switching between different embedding models as long as they have the same dimension.
	if err := validateDimensionLock(ctx, pool, vectorDim); err != nil {
		pool.Close()
		return nil, fmt.Errorf("dimension lock: %w", err)
	}

	// Store embedding model metadata for audit purposes (dimension is what matters for storage).
	if err := storeEmbeddingModelMetadata(ctx, pool, expectedModel); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store embedding model metadata: %w", err)
	}

	return pool, nil
}

const (
	connectRetryWindow  = 2 * time.Minute
	connectRetryInitial = 1 * time.Second
	connectRetryMax     = 10 * time.Second
)

const (
	// schemaLockKey names the advisory lock that serialises schema setup. It is
	// database-wide, so it only ever contends with another CortexIQ on the same
	// database, and any fixed number would do; this one is "cortexiq" read as a
	// base-36 number so it is recognisable in pg_locks.
	schemaLockKey int64 = 994294737458

	// schemaLockWait bounds how long a starting process waits for another one
	// that is migrating. Migrations here take well under a second; this is long
	// enough for a large index build and short enough that a stuck holder shows
	// up as a start-up error instead of a hang.
	schemaLockWait = 5 * time.Minute
)

// withSchemaLock runs fn while holding the schema advisory lock. The lock is
// session-level, so it is taken and released on one dedicated connection; fn does
// its work on other connections, which is fine because the lock is only a token.
// If the process dies the connection drops and Postgres releases the lock itself.
func withSchemaLock(ctx context.Context, pool *pgxpool.Pool, fn func() error) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection for the schema lock: %w", err)
	}
	defer conn.Release()

	waitCtx, cancel := context.WithTimeout(ctx, schemaLockWait)
	defer cancel()
	if _, err := conn.Exec(waitCtx, "SELECT pg_advisory_lock($1)", schemaLockKey); err != nil {
		// A cancelled wait can leave the request half-delivered, so the lock may or
		// may not have been taken. Closing the session settles it either way.
		_ = conn.Conn().Close(context.WithoutCancel(ctx))
		return fmt.Errorf("wait for the schema lock (another instance may be migrating): %w", err)
	}
	defer func() {
		if _, err := conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", schemaLockKey); err != nil {
			// Do not hand a connection that may still hold the lock back to the pool.
			_ = conn.Conn().Close(context.WithoutCancel(ctx))
		}
	}()

	return fn()
}

// gooseMu guards goose's package-level state (base file system, logger and
// dialect). The advisory lock orders two callers in time, but it is a database
// round trip and gives Go no happens-before edge, so two Opens in one process
// (as the tests do, and as a future embedder might) still race on those
// variables. The mutex is what makes that safe, and it is only ever taken while
// holding the advisory lock, so the two cannot deadlock.
var gooseMu sync.Mutex

// setupSchema brings the schema up to date: goose migrations, the vector
// dimension on columns that were created unconstrained, and the HNSW indexes.
// Callers hold the schema lock.
func setupSchema(ctx context.Context, pool *pgxpool.Pool, sqlDB *sql.DB, vectorDim int) error {
	gooseMu.Lock()
	defer gooseMu.Unlock()

	goose.SetBaseFS(embedMigrations)
	goose.SetLogger(discardLogger{})
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose.SetDialect: %w", err)
	}

	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return fmt.Errorf("goose.Up: %w", err)
	}

	// Set vector dimensions on columns that use unconstrained `vector` type.
	vectorColumns := []struct {
		table  string
		column string
	}{
		{"episodes", "embedding"},
		{"facts", "embedding"},
		{"embedding_cache", "embedding"},
	}
	for _, vc := range vectorColumns {
		var currentDim int
		if err := pool.QueryRow(ctx,
			"SELECT atttypmod FROM pg_attribute a JOIN pg_class c ON a.attrelid = c.oid WHERE c.relname = $1 AND a.attname = $2",
			vc.table, vc.column,
		).Scan(&currentDim); err != nil {
			return fmt.Errorf("check vector dim %s.%s: %w", vc.table, vc.column, err)
		}
		if currentDim == -1 {
			alterDDL := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE vector(%d)", vc.table, vc.column, vectorDim)
			if _, err := sqlDB.ExecContext(ctx, alterDDL); err != nil {
				return fmt.Errorf("alter vector dim %s.%s: %w", vc.table, vc.column, err)
			}
		}
	}

	// Create HNSW indexes for vector columns (idempotent).
	hnswIndexes := []struct {
		table  string
		column string
	}{
		{"episodes", "embedding"},
		{"facts", "embedding"},
	}
	for _, idx := range hnswIndexes {
		indexName := idx.table + "_embedding_hnsw_idx"
		var exists bool
		if err := pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = $1)", indexName,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check hnsw index %s: %w", indexName, err)
		}
		if !exists {
			ddl := fmt.Sprintf(
				"CREATE INDEX %s ON %s USING hnsw (%s vector_cosine_ops)",
				indexName, idx.table, idx.column,
			)
			if _, err := sqlDB.ExecContext(ctx, ddl); err != nil {
				return fmt.Errorf("create hnsw index %s: %w", indexName, err)
			}
		}
	}
	return nil
}

// pingWithRetry calls ping until it succeeds, the window elapses or ctx is
// cancelled, doubling the wait between attempts up to max. It returns the last
// ping error when it gives up.
func pingWithRetry(ctx context.Context, ping func(context.Context) error, window, initial, max time.Duration) error {
	deadline := time.Now().Add(window)
	delay := initial
	for attempt := 1; ; attempt++ {
		err := ping(ctx)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil || time.Now().Add(delay).After(deadline) {
			return err
		}
		log.Printf("database not reachable (attempt %d), retrying in %s: %v", attempt, delay, err)
		select {
		case <-ctx.Done():
			return err
		case <-time.After(delay):
		}
		if delay *= 2; delay > max {
			delay = max
		}
	}
}

// validateDimensionLock ensures the vector dimension stored in the database matches the config.
// This is the actual storage constraint; the specific embedding model can vary as long as dimensions match.
func validateDimensionLock(ctx context.Context, pool *pgxpool.Pool, expectedDim int) error {
	var storedDimStr string
	err := pool.QueryRow(ctx,
		"SELECT value FROM settings WHERE key = 'vector_dimension'",
	).Scan(&storedDimStr)

	if err != nil {
		// No row yet — store the expected dimension as string.
		_, err := pool.Exec(ctx,
			"INSERT INTO settings (key, value) VALUES ('vector_dimension', $1) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()",
			fmt.Sprintf("%d", expectedDim),
		)
		return err
	}

	if storedDimStr != "" {
		storedDim, err := strconv.Atoi(storedDimStr)
		if err != nil {
			return fmt.Errorf("invalid stored dimension value: %w", err)
		}
		if storedDim != expectedDim {
			return fmt.Errorf("vector dimension mismatch: database has %d, config expects %d. You can switch between different embedding models as long as they output the same dimension. Change CORTEXIQ_VECTOR_DIM to match the database, or delete the database and restart", storedDim, expectedDim)
		}
	}

	return nil
}

// storeEmbeddingModelMetadata records which embedding model is being used, for audit/monitoring purposes.
// This does not affect storage constraints (which are based on vector dimension only).
func storeEmbeddingModelMetadata(ctx context.Context, pool *pgxpool.Pool, model string) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO settings (key, value) VALUES ('embedding_model', $1) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()",
		model,
	)
	return err
}
