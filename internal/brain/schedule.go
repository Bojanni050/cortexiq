package brain

import (
	"context"
	"sort"

	"github.com/Bojanni050/cortexiq/internal/schedule"
)

// ScheduleConfig returns the stored consolidation schedule, or the default
// (every five minutes) when nothing has been saved yet.
func (b *Brain) ScheduleConfig(ctx context.Context) (schedule.Config, error) {
	return schedule.NewStore(b.pool).Load(ctx)
}

// SeedSchedule stores the given schedule only where no value exists yet, so the
// CLI flag remains the initial value without overwriting a dashboard choice.
func (b *Brain) SeedSchedule(ctx context.Context, cfg schedule.Config) error {
	return schedule.NewStore(b.pool).Seed(ctx, cfg)
}

// SetScheduleConfig validates and stores the schedule, then wakes the running
// consolidator so the change applies without a restart.
func (b *Brain) SetScheduleConfig(ctx context.Context, cfg schedule.Config) error {
	if err := schedule.NewStore(b.pool).Save(ctx, cfg); err != nil {
		return err
	}
	select {
	case b.scheduleChanged <- struct{}{}:
	default:
	}
	return nil
}

// ScheduleChanged fires when SetScheduleConfig stored a new schedule. The
// channel is buffered, so a change never blocks the writer and a burst of
// changes collapses into one wake-up.
func (b *Brain) ScheduleChanged() <-chan struct{} { return b.scheduleChanged }

// EpisodeSignal fires when new episodes were remembered. The consolidator
// treats it as a wake-up and asks DrainEpisodeNamespaces for the work.
func (b *Brain) EpisodeSignal() <-chan struct{} { return b.episodeSignal }

// DrainEpisodeNamespaces returns the namespaces that received an episode since
// the last call and clears the set. Returns nil when nothing is pending, so the
// caller can skip a run entirely.
func (b *Brain) DrainEpisodeNamespaces() []int64 {
	b.episodeMu.Lock()
	defer b.episodeMu.Unlock()
	if len(b.episodeNS) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(b.episodeNS))
	for id := range b.episodeNS {
		ids = append(ids, id)
	}
	b.episodeNS = nil
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// noteEpisode records a namespace that just received an episode. Repeats in the
// same namespace collapse, which is what makes a burst of remembers one run.
func (b *Brain) noteEpisode(namespaceID int64) {
	b.episodeMu.Lock()
	if b.episodeNS == nil {
		b.episodeNS = map[int64]struct{}{}
	}
	b.episodeNS[namespaceID] = struct{}{}
	b.episodeMu.Unlock()

	select {
	case b.episodeSignal <- struct{}{}:
	default:
	}
}
