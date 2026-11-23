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
	// Now fires when someone asked for a round right away (the dashboard's Run
	// now button). It runs every configured namespace, whatever the mode: a
	// manual round is not an event and not a tick. nil disables it.
	Now func() <-chan struct{}
	// Pending returns the namespaces that still have work waiting behind a
	// checkpoint. A run that fails on one batch leaves the work ahead of it
	// untouched, and waiting for the next tick means waiting up to the whole
	// interval — a day on a slow schedule. nil disables the early retry.
	Pending func(ctx context.Context) ([]int64, error)
	// RetryDelay is how long after such a run the pending work is tried again.
	// One retry per cycle, never a loop: a batch that fails deterministically
	// must not turn into an LLM call every few minutes.
	RetryDelay time.Duration
	// NewTicker makes the periodic timer; nil means time.NewTicker. Tests
	// replace it to get a fast clock without waiting minutes.
	NewTicker func(time.Duration) *time.Ticker
	// Status reports the loop's live state every time it changes: whether a run
	// is in progress and when the next one is due. The timer exists in this
	// process only, so the dashboard cannot read the next tick from the
	// database; it is told here instead. nil disables it.
	Status func(Status)
	// Logf reports what happened; nil discards.
	Logf func(format string, args ...any)

	// status is the loop's own view, kept on the scheduler so run() can update
	// it without threading it through every call site.
	status Status
}

// Status is what the loop is doing right now. It is not stored: a restarted
// process arms a fresh timer, so a saved next run would be a lie.
type Status struct {
	Mode     Mode
	Interval time.Duration
	// Running is true while Consolidate is executing.
	Running bool
	// RunningSince is when the current batch of namespaces started.
	RunningSince time.Time
	// Namespace is the namespace being consolidated, 0 when idle.
	Namespace int64
	// NextRun is when the timer fires; zero when no timer is armed.
	NextRun time.Time
	// RetryAt is when a run is retried because work is waiting behind a failed
	// batch; zero when no retry is armed.
	RetryAt time.Time
}

// publish hands the loop's state to the Status hook. Every change goes through
// it, so a reader never sees a half-updated view.
func (s *Scheduler) publish(change func(*Status)) {
	if change != nil {
		change(&s.status)
	}
	if s.Status != nil {
		s.Status(s.status)
	}
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
	s.status = Status{Mode: cfg.Mode, Interval: cfg.Interval()}

	episodes := s.Episodes()
	changes := s.Changes()
	// A nil channel blocks forever in select, which is exactly "disabled".
	var now <-chan struct{}
	if s.Now != nil {
		now = s.Now()
	}

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
		// The countdown the dashboard shows starts at the moment the timer is
		// armed, which is the only place the next tick is known.
		s.publish(func(st *Status) {
			st.Mode = cfg.Mode
			st.Interval = cfg.Interval()
			st.NextRun = time.Time{}
		})
		if cfg.UsesTimer() {
			ticker = newTicker(cfg.Interval())
			ticks = ticker.C
			s.publish(func(st *Status) { st.NextRun = time.Now().Add(cfg.Interval()) })
		}
	}
	arm()
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()

	s.logf("schedule: mode=%s interval=%s", cfg.Mode, cfg.Interval())

	// retry is armed after a run that left work behind, and fires once.
	var retryT *time.Timer
	var retryC <-chan time.Time
	armRetry := func(ctx context.Context) {
		if s.Pending == nil || s.RetryDelay <= 0 {
			return
		}
		// A retry armed by the previous run is spent by now; the dashboard
		// should stop counting it down while this one is decided.
		s.publish(func(st *Status) { st.RetryAt = time.Time{} })
		ids, err := s.Pending(ctx)
		if err != nil {
			s.logf("schedule: cannot read pending work: %v", err)
			return
		}
		if len(ids) == 0 {
			return
		}
		if retryT != nil {
			retryT.Stop()
		}
		retryT = time.NewTimer(s.RetryDelay)
		retryC = retryT.C
		s.publish(func(st *Status) { st.RetryAt = time.Now().Add(s.RetryDelay) })
		s.logf("schedule: %d namespace(s) still have work waiting; retrying in %s", len(ids), s.RetryDelay)
	}
	defer func() {
		if retryT != nil {
			retryT.Stop()
		}
	}()

	s.runAll(ctx)
	armRetry(ctx)

	for {
		// Events that arrived while a run was going become the next run. The
		// set collapses repeats, so a burst is one run per namespace.
		ids := s.drain()
		if len(ids) > 0 && cfg.TriggersOnEvents() {
			s.runNamespaces(ctx, ids)
			armRetry(ctx)
			continue
		}

		select {
		case <-ctx.Done():
			s.logf("schedule: stopped")
			return nil
		case <-ticks:
			// The ticker keeps its own clock: the following tick is one
			// interval after this one, not after the run that follows it. A run
			// that overran leaves its tick waiting, and then the next round
			// starts as soon as this one ends.
			s.publish(func(st *Status) {
				if st.NextRun.IsZero() {
					return
				}
				st.NextRun = st.NextRun.Add(cfg.Interval())
				if st.NextRun.Before(time.Now()) {
					st.NextRun = time.Now()
				}
			})
			s.runAll(ctx)
			armRetry(ctx)
		case <-retryC:
			// One attempt: whatever is still behind after this waits for the next
			// tick, so a permanently failing batch cannot spin.
			retryC = nil
			s.publish(func(st *Status) { st.RetryAt = time.Time{} })
			left, err := s.Pending(ctx)
			if err != nil {
				s.logf("schedule: cannot read pending work: %v", err)
				continue
			}
			if len(left) == 0 {
				continue
			}
			s.logf("schedule: retrying %d namespace(s) with work still waiting", len(left))
			s.runNamespaces(ctx, left)
		case <-now:
			s.logf("schedule: manual round requested")
			s.runAll(ctx)
			armRetry(ctx)
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
			s.logf("schedule: namespace %d is outside the configured set; skipping run", id)
			continue
		}
		run = append(run, id)
	}
	s.run(ctx, run)
}

func (s *Scheduler) run(ctx context.Context, ids []int64) {
	if s.Consolidate == nil || len(ids) == 0 {
		return
	}
	// One batch of namespaces counts as one run for a reader: the run rows are
	// written per namespace, but "is it running" is a question about the loop.
	s.publish(func(st *Status) {
		st.Running = true
		st.RunningSince = time.Now()
		st.Namespace = ids[0]
	})
	defer s.publish(func(st *Status) {
		st.Running = false
		st.RunningSince = time.Time{}
		st.Namespace = 0
	})
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		s.publish(func(st *Status) { st.Namespace = id })
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
