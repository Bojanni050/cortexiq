// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"testing"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
)

// A write through the dashboard must reach the consolidator in the other
// process, and the database is the only thing the two share. The test stands in
// for that deployment: it marks the namespace as fully caught up first, so the
// listener's catch-up has nothing to report and the only thing that can wake it
// is the notification itself.
func TestIntegrationNotifyConsolidatorWakesTheListener(t *testing.T) {
	e := newEnv(t, nil)

	out, err := e.b.RememberWithOutcome(e.ctx, e.ns,
		"The dashboard stored this memory and expects the consolidator to notice without waiting for a timer.",
		RememberOptions{})
	if err != nil {
		t.Fatalf("RememberWithOutcome: %v", err)
	}
	// The local write rang the in-process bell as well; that path is already
	// covered elsewhere and would hide whether the notification works.
	e.b.DrainEpisodeNamespaces()

	if _, err := e.b.GetOrCreateConsolidationProgress(e.ctx, e.nsID); err != nil {
		t.Fatalf("GetOrCreateConsolidationProgress: %v", err)
	}
	cp := models.ConsolidationProgress{NamespaceID: e.nsID, LastEpisodeID: out.ID}
	if err := e.b.SaveConsolidationProgress(e.ctx, cp); err != nil {
		t.Fatalf("SaveConsolidationProgress: %v", err)
	}

	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()
	go e.b.WatchEpisodeSignals(ctx)

	// The listener needs a moment to establish LISTEN; a notification published
	// before that is gone by design, which is why the tick exists. Sending
	// again is how the real callers behave — every write rings once.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := e.b.NotifyConsolidator(e.ctx, out.ID); err != nil {
			t.Fatalf("NotifyConsolidator: %v", err)
		}
		for _, got := range e.b.DrainEpisodeNamespaces() {
			if got == e.nsID {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("the notification did not reach the listener's drain within 10s")
}

// An id that does not exist must not fail the write that came before it: the
// caller only learns that nothing could be signalled.
func TestIntegrationNotifyConsolidatorIgnoresAnUnknownEpisode(t *testing.T) {
	e := newEnv(t, nil)

	if err := e.b.NotifyConsolidator(e.ctx, 0); err != nil {
		t.Fatalf("NotifyConsolidator(0) = %v, want nil", err)
	}
	if err := e.b.NotifyConsolidator(e.ctx, 1<<40); err != nil {
		t.Fatalf("NotifyConsolidator(unknown) = %v, want nil", err)
	}
	if got := e.b.DrainEpisodeNamespaces(); len(got) != 0 {
		t.Fatalf("an unknown episode woke %v", got)
	}
}
