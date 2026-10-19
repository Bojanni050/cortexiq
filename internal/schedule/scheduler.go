package schedule

import (
	"context"
	"sort"
	"time"
)

// Scheduler runs consolidation on the mode and interval in Config. It owns the
// loop only: the work is supplied through hooks, so it can run against a brain
// or a fake in a test.
type Scheduler struct {
	// Initial is the schedule used until a stored change arrives. The caller
	// loads it from the store (or from its flag when the store cannot be read).
	Initial Config

	// Consolidate runs the pipeline for one namespace.
	Consolidate func(ctx context.Context, namespaceID int64) error
	// Namespaces resolves the configured namespace set for a timer run. Event
	// runs use it as a filter: a namespace outside the set is not consolidated.
	Namespaces func(ctx context.Context) ([]int64, error)
	// Config loads the stored schedule after a change. On error the current
	// schedule stays in force.
	Config func(ctx context.Context) (Config, error)
	// Episodes fires when new episodes were remembered; it only wakes the
	// loop, the namespaces come from Drain.
	Episodes func() <-chan struct{}
	// Drain returns the namespaces remembered since the last call and clears
	// the set.
	Drain func() []int64
	// Changes fires when the stored schedule changed.
	Changes func() <-chan struct{}
	// NewTicker makes the periodic timer; nil means time.NewTicker. Tests
	// replace it to get a fast clock without waiting minutes.
	NewTicker func(time.Duration) *time.Ticker
	// Logf reports what happened; nil discards.
	Logf func(format string, args ...any)
}

// Run blocks until ctx is cancelled. It always does one catch-up run first:
// episodes may have been written by another process (the stdio MCP server) or
// while this one was down, and in events-only mode nothing else would pick
// them up.
func (s *Scheduler) Run(ctx context.Context) error {
	cfg := s.Initial
	if err := cfg.Validate(); err != nil {
		s.logf("schedule: invalid initial config (%v); falling back to %s every %s", err, DefaultConfig().Mode, DefaultInterval)
		cfg = DefaultConfig()
	}

	episodes := s.Episodes()
	changes := s.Changes()

	var ticker *time.Ticker
	var ticks <-chan time.Time
	newTicker := s.NewTicker
	if newTicker == nil {
		newTicker = time.NewTicker
	}
	arm := func() {
		if ticker != nil {
			ticker.Stop()
			ticker, ticks = nil, nil
		}
		if cfg.UsesTimer() {
			ticker = newTicker(cfg.Interval())
			ticks = ticker.C
		}
	}
	arm()
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()

	s.logf("schedule: mode=%s interval=%s", cfg.Mode, cfg.Interval())

	s.runAll(ctx)

	for {
		// Events that arrived while a run was going become the next run. The
		// set collapses repeats, so a burst is one run per namespace.
		ids := s.drain()
		if len(ids) > 0 && cfg.TriggersOnEvents() {
			s.runNamespaces(ctx, ids)
			continue
		}

		select {
		case <-ctx.Done():
			s.logf("schedule: stopped")
			return nil
		case <-ticks:
			s.runAll(ctx)
		case <-episodes:
			// Only a wake-up; the next iteration drains the namespaces.
		case <-changes:
			next, err := s.Config(ctx)
			if err != nil {
				s.logf("schedule: cannot reload schedule, keeping %s: %v", cfg.Mode, err)
				continue
			}
			if err := next.Validate(); err != nil {
				s.logf("schedule: stored schedule is invalid, keeping %s: %v", cfg.Mode, err)
				continue
			}
			cfg = next
			arm()
			s.logf("schedule: now mode=%s interval=%s", cfg.Mode, cfg.Interval())
		}
	}
}

// runAll consolidates every configured namespace: a timer run or the start-up
// catch-up.
func (s *Scheduler) runAll(ctx context.Context) {
	if s.Namespaces == nil {
		return
	}
	ids, err := s.Namespaces(ctx)
	if err != nil {
		s.logf("schedule: cannot resolve namespaces: %v", err)
		return
	}
	s.run(ctx, ids)
}

// runNamespaces consolidates the namespaces that just received an episode,
// limited to the configured set.
func (s *Scheduler) runNamespaces(ctx context.Context, ids []int64) {
	allowed := map[int64]bool{}
	if s.Namespaces != nil {
		set, err := s.Namespaces(ctx)
		if err != nil {
			s.logf("schedule: cannot resolve namespaces: %v", err)
			return
		}
		for _, id := range set {
			allowed[id] = true
		}
	}
	var run []int64
	for _, id := range ids {
		if len(allowed) > 0 && !allowed[id] {
			s.logf("schedule: namespace %d is outside the configured set; skipping event run", id)
			continue
		}
		run = append(run, id)
	}
	s.run(ctx, run)
}

func (s *Scheduler) run(ctx context.Context, ids []int64) {
	if s.Consolidate == nil {
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		if err := s.Consolidate(ctx, id); err != nil {
			s.logf("schedule: consolidation failed for namespace %d: %v", id, err)
		}
	}
}

// drain reads the pending namespaces, sorted and without repeats: a burst of
// remembers in one namespace is one run, whatever the source reports.
func (s *Scheduler) drain() []int64 {
	if s.Drain == nil {
		return nil
	}
	ids := s.Drain()
	if len(ids) < 2 {
		return ids
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := ids[:1]
	for _, id := range ids[1:] {
		if id != out[len(out)-1] {
			out = append(out, id)
		}
	}
	return out
}

func (s *Scheduler) logf(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
	}
}
