package schedule

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	keyMode     = "schedule.mode"
	keyInterval = "schedule.interval_seconds"
)

// Store reads and writes the consolidation schedule in the settings table.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a store on the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Load returns the stored schedule. Missing keys fall back to DefaultConfig,
// so an install that never opened the settings page keeps the old behaviour.
func (s *Store) Load(ctx context.Context) (Config, error) {
	cfg := DefaultConfig()

	rows, err := s.pool.Query(ctx,
		`SELECT key, value FROM settings WHERE key = ANY($1)`,
		[]string{keyMode, keyInterval})
	if err != nil {
		return Config{}, fmt.Errorf("settings: read schedule: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return Config{}, fmt.Errorf("settings: scan schedule: %w", err)
		}
		switch key {
		case keyMode:
			cfg.Mode = Mode(value)
		case keyInterval:
			n, err := strconv.Atoi(value)
			if err != nil {
				return Config{}, fmt.Errorf("settings: %s is %q, not a number", key, value)
			}
			cfg.IntervalSeconds = n
		}
	}
	if err := rows.Err(); err != nil {
		return Config{}, fmt.Errorf("settings: read schedule: %w", err)
	}
	return cfg, nil
}

// Save validates and stores the schedule. Both keys are written together, so a
// reader never sees a new mode with an old interval.
func (s *Store) Save(ctx context.Context, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("settings: write schedule: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const upsert = `INSERT INTO settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`
	if _, err := tx.Exec(ctx, upsert, keyMode, string(cfg.Mode)); err != nil {
		return fmt.Errorf("settings: write mode: %w", err)
	}
	if _, err := tx.Exec(ctx, upsert, keyInterval, strconv.Itoa(cfg.IntervalSeconds)); err != nil {
		return fmt.Errorf("settings: write interval: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("settings: commit schedule: %w", err)
	}
	return nil
}

// Seed stores the given schedule only where no value exists yet. It lets a
// service start with its --consolidate-interval flag as the initial value
// without ever overwriting a choice made in the dashboard.
func (s *Store) Seed(ctx context.Context, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	const insert = `INSERT INTO settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO NOTHING`
	if _, err := s.pool.Exec(ctx, insert, keyMode, string(cfg.Mode)); err != nil {
		return fmt.Errorf("settings: seed mode: %w", err)
	}
	if _, err := s.pool.Exec(ctx, insert, keyInterval, strconv.Itoa(cfg.IntervalSeconds)); err != nil {
		return fmt.Errorf("settings: seed interval: %w", err)
	}
	return nil
}
