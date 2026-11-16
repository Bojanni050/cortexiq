// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"
)

// episodeNotifyChannel is the Postgres channel that wakes a consolidator
// living in another process. The payload is the namespace id.
//
// The split deployment runs the dashboard and the consolidator as separate
// processes that share nothing but the database. A write through the dashboard
// therefore cannot ring the in-process bell: without this notification it
// waited for the next timer tick, which on the configured schedule is two
// hours. Postgres is the one channel both sides are known to have.
const episodeNotifyChannel = "cortexiq_episode"

// NotifyConsolidator tells whichever process runs consolidation that this
// episode is waiting. It is a courtesy, not a guarantee: the listener may be
// restarting at that moment, and the timer is still the safety net. A caller
// already inside the consolidator's own process does not need it, because the
// write path rings the in-process bell itself.
func (b *Brain) NotifyConsolidator(ctx context.Context, episodeID int64) error {
	if episodeID <= 0 {
		return nil
	}
	// One statement: the namespace comes from the row the caller just wrote, so
	// a caller never has to resolve a slug it already handed to Remember.
	if _, err := b.pool.Exec(ctx,
		`SELECT pg_notify($1, namespace_id::text) FROM episodes WHERE id = $2`,
		episodeNotifyChannel, episodeID); err != nil {
		return fmt.Errorf("notify consolidator of episode %d: %w", episodeID, err)
	}
	return nil
}

// WatchEpisodeSignals feeds notifications from other processes into the same
// path a local write uses, so the consolidator drains the affected namespaces
// instead of waiting for a tick. It blocks until ctx is cancelled and
// reconnects when the database drops the connection.
//
// It is started only by the process that runs consolidation; anywhere else the
// notifications have no reader, and the timer remains the only trigger.
func (b *Brain) WatchEpisodeSignals(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := b.listenEpisodeSignals(ctx)
		if ctx.Err() != nil {
			return
		}
		log.Printf("episode signals: listener stopped: %v; retrying in %s", err, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < time.Minute {
			backoff *= 2
		}
	}
}

func (b *Brain) listenEpisodeSignals(ctx context.Context) error {
	pooled, err := b.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	// A listening connection must not sit in the pool: it is idle for hours and
	// would occupy a slot a query needs. Hijack hands it over for good.
	conn := pooled.Hijack()
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()

	if _, err := conn.Exec(ctx, "LISTEN "+episodeNotifyChannel); err != nil {
		return fmt.Errorf("listen on %s: %w", episodeNotifyChannel, err)
	}
	// A notification published while nobody was listening is gone. Catching up
	// here is what makes a restart or a dropped connection cost one query
	// instead of a full interval of waiting.
	b.catchUpPendingWork(ctx)
	log.Printf("episode signals: listening on %s", episodeNotifyChannel)

	for {
		note, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		id, err := strconv.ParseInt(note.Payload, 10, 64)
		if err != nil || id <= 0 {
			log.Printf("episode signals: ignoring payload %q", note.Payload)
			continue
		}
		b.noteEpisode(id)
	}
}

// catchUpPendingWork notes the namespaces that already have work behind their
// checkpoint, so a signal lost to a restart does not have to wait for the tick.
func (b *Brain) catchUpPendingWork(ctx context.Context) {
	ids, err := b.NamespacesWithPendingWork(ctx)
	if err != nil {
		log.Printf("episode signals: cannot read pending work: %v", err)
		return
	}
	if len(ids) == 0 {
		return
	}
	for _, id := range ids {
		b.noteEpisode(id)
	}
	log.Printf("episode signals: %d namespace(s) already had work waiting", len(ids))
}
