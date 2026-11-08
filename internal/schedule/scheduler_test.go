package schedule

import (
	"context"
	"sync"
	"testing"
	"time"
)

// harness is a scheduler with every hook faked: it records what ran, keeps the
// pending set a test writes, and can hold a run open to test coalescing.
type harness struct {
	mu         sync.Mutex
	calls      []int64
	namespaces []int64
	pending    []int64
	cfg        Config

	eps     chan struct{}
	changes chan struct{}
	ran     chan int64
	block   chan struct{}

	// work is what Pending reports: the namespaces a failed run left behind.
	work      []int64
	tickEvery time.Duration
}

func newHarness(cfg Config, namespaces ...int64) *harness {
	return &harness{
		cfg:        cfg,
		namespaces: namespaces,
		eps:        make(chan struct{}, 1),
		changes:    make(chan struct{}, 1),
		ran:        make(chan int64, 100),
	}
}

func (h *harness) consolidate(ctx context.Context, id int64) error {
	h.mu.Lock()
	block := h.block
	h.mu.Unlock()
	if block != nil {
		<-block
	}
	h.mu.Lock()
	h.calls = append(h.calls, id)
	h.mu.Unlock()
	select {
	case h.ran <- id:
	default:
	}
	return nil
}

func (h *harness) drain() []int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := h.pending
	h.pending = nil
	return out
}

func (h *harness) addPending(ids ...int64) {
	h.mu.Lock()
	h.pending = append(h.pending, ids...)
	h.mu.Unlock()
	select {
	case h.eps <- struct{}{}:
	default:
	}
}

func (h *harness) setConfig(cfg Config) {
	h.mu.Lock()
	h.cfg = cfg
	h.mu.Unlock()
	select {
	case h.changes <- struct{}{}:
	default:
	}
}

func (h *harness) setWork(ids ...int64) {
	h.mu.Lock()
	h.work = ids
	h.mu.Unlock()
}

func (h *harness) setTickEvery(d time.Duration) {
	h.mu.Lock()
	h.tickEvery = d
	h.mu.Unlock()
}

func (h *harness) setBlock(ch chan struct{}) {
	h.mu.Lock()
	h.block = ch
	h.mu.Unlock()
}

func (h *harness) scheduler() *Scheduler {
	return &Scheduler{
		Initial:     h.cfg,
		Consolidate: h.consolidate,
		Namespaces: func(ctx context.Context) ([]int64, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			return append([]int64(nil), h.namespaces...), nil
		},
		Config: func(ctx context.Context) (Config, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.cfg, nil
		},
		Episodes: func() <-chan struct{} { return h.eps },
		Drain:    h.drain,
		Changes:  func() <-chan struct{} { return h.changes },
		Pending: func(context.Context) ([]int64, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			return append([]int64(nil), h.work...), nil
		},
		NewTicker: func(time.Duration) *time.Ticker {
			h.mu.Lock()
			d := h.tickEvery
			h.mu.Unlock()
			if d == 0 {
				d = 10 * time.Millisecond
			}
			return time.NewTicker(d)
		},
	}
}

func (h *harness) snapshot() []int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]int64(nil), h.calls...)
}

// run starts the scheduler and stops it when the test ends.
func run(t *testing.T, s *Scheduler) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("scheduler did not stop after cancel")
		}
	})
	return cancel
}

func waitCall(t *testing.T, h *harness) int64 {
	t.Helper()
	select {
	case id := <-h.ran:
		return id
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not run within 2s")
		return 0
	}
}

func expectCalls(t *testing.T, h *harness, want ...int64) {
	t.Helper()
	got := h.snapshot()
	if len(got) != len(want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("runs = %v, want %v", got, want)
		}
	}
}

func TestSchedulerEventsTriggerOneRunForTheNamespace(t *testing.T) {
	h := newHarness(Config{Mode: ModeEvents, IntervalSeconds: 300}, 1, 2)
	run(t, h.scheduler())

	// One catch-up run over the configured set, so nothing that was written
	// while nothing was listening stays behind.
	waitCall(t, h)
	waitCall(t, h)
	expectCalls(t, h, 1, 2)

	h.addPending(2)
	if id := waitCall(t, h); id != 2 {
		t.Fatalf("event run = %d, want 2", id)
	}
	expectCalls(t, h, 1, 2, 2)

	// An events-only schedule has no timer, so nothing else may run.
	select {
	case id := <-h.ran:
		t.Fatalf("unexpected run for namespace %d in events-only mode", id)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestSchedulerCoalescesABurstDuringARun(t *testing.T) {
	h := newHarness(Config{Mode: ModeEvents, IntervalSeconds: 300}, 1)
	gate := make(chan struct{})
	h.setBlock(gate)
	run(t, h.scheduler())

	// The catch-up run is now held open; remembers keep arriving meanwhile.
	time.Sleep(20 * time.Millisecond)
	h.addPending(1)
	h.addPending(1)
	h.addPending(1)
	close(gate)

	// The catch-up finishes, then exactly one follow-up run drains the burst.
	waitCall(t, h)
	if id := waitCall(t, h); id != 1 {
		t.Fatalf("burst run = %d, want 1", id)
	}
	expectCalls(t, h, 1, 1)

	select {
	case id := <-h.ran:
		t.Fatalf("burst produced an extra run for namespace %d", id)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestSchedulerSkipsEventsOutsideTheConfiguredSet(t *testing.T) {
	h := newHarness(Config{Mode: ModeEvents, IntervalSeconds: 300}, 1)
	run(t, h.scheduler())
	waitCall(t, h)

	h.addPending(9)
	select {
	case id := <-h.ran:
		t.Fatalf("namespace %d is outside the configured set but ran", id)
	case <-time.After(150 * time.Millisecond):
	}
	expectCalls(t, h, 1)
}

func TestSchedulerSwitchesModeWhenTheSettingChanges(t *testing.T) {
	h := newHarness(Config{Mode: ModeEvents, IntervalSeconds: 300}, 1)
	run(t, h.scheduler())
	waitCall(t, h)

	// Switching to a timer must start runs without a restart.
	h.setConfig(Config{Mode: ModeInterval, IntervalSeconds: 60})
	waitCall(t, h)
	waitCall(t, h)
	if calls := h.snapshot(); len(calls) < 3 {
		t.Fatalf("runs after switching to interval = %v, want repeated runs", calls)
	}

	// And a switch back to events stops the timer.
	h.setConfig(Config{Mode: ModeEvents, IntervalSeconds: 300})
	time.Sleep(30 * time.Millisecond)
	before := len(h.snapshot())
	time.Sleep(100 * time.Millisecond)
	if after := len(h.snapshot()); after != before {
		t.Fatalf("events-only mode still ran %d times", after-before)
	}
}

// A run that fails on one batch leaves the work behind it untouched. Waiting for
// the next tick means waiting the whole interval — a day on a slow schedule — so
// the scheduler tries once more, shortly, and then stops.
func TestSchedulerRetriesPendingWorkOnce(t *testing.T) {
	h := newHarness(ConfigFromInterval(time.Hour), 1, 2)
	h.setTickEvery(time.Hour) // only the retry can produce the third run
	h.setWork(2)              // namespace 2 still has work after the run

	s := h.scheduler()
	s.RetryDelay = 20 * time.Millisecond
	run(t, s)

	if id := waitCall(t, h); id != 1 {
		t.Fatalf("first run = %d, want 1", id)
	}
	if id := waitCall(t, h); id != 2 {
		t.Fatalf("second run = %d, want 2", id)
	}
	if id := waitCall(t, h); id != 2 {
		t.Fatalf("retry ran namespace %d, want 2", id)
	}

	// No third attempt: the retry happens once, so a batch that keeps failing
	// cannot turn into a model call every few minutes.
	select {
	case id := <-h.ran:
		t.Fatalf("a third run (%d) happened; the retry must not loop", id)
	case <-time.After(80 * time.Millisecond):
	}
}

// Without work waiting there is nothing to retry, and arming a timer anyway would
// make every quiet tick look busy.
func TestSchedulerDoesNotRetryWhenNothingIsWaiting(t *testing.T) {
	h := newHarness(ConfigFromInterval(time.Hour), 1)
	h.setTickEvery(time.Hour)

	s := h.scheduler()
	s.RetryDelay = 20 * time.Millisecond
	run(t, s)

	if id := waitCall(t, h); id != 1 {
		t.Fatalf("first run = %d, want 1", id)
	}
	select {
	case id := <-h.ran:
		t.Fatalf("namespace %d ran although nothing was pending", id)
	case <-time.After(80 * time.Millisecond):
	}
}
