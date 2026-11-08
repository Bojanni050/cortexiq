// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
)

// WriteWorkerConfig tunes the deferred write worker. The zero value is replaced
// by DefaultWriteWorkerConfig.
type WriteWorkerConfig struct {
	// Poll is how often the worker looks for work nobody woke it for: a job
	// queued by another process, or one left behind by a crash.
	Poll time.Duration
	// StaleAfter is when a running job is assumed to belong to a dead worker.
	StaleAfter time.Duration
	// Retention is how long a finished job stays readable before it is pruned.
	Retention time.Duration
	// MaxAttempts bounds retries of a transient failure.
	MaxAttempts int
	// RetryDelay is the base backoff; attempt n waits n×RetryDelay.
	RetryDelay time.Duration
}

// DefaultWriteWorkerConfig is what production runs with: a two second poll is
// invisible next to the request it replaces, ten minutes is far beyond a cold
// embedding call, and a week is long enough to answer "what happened to my
// write?".
func DefaultWriteWorkerConfig() WriteWorkerConfig {
	return WriteWorkerConfig{
		Poll:        2 * time.Second,
		StaleAfter:  10 * time.Minute,
		Retention:   7 * 24 * time.Hour,
		MaxAttempts: 3,
		RetryDelay:  30 * time.Second,
	}
}

func (c WriteWorkerConfig) withDefaults() WriteWorkerConfig {
	d := DefaultWriteWorkerConfig()
	if c.Poll > 0 {
		d.Poll = c.Poll
	}
	if c.StaleAfter > 0 {
		d.StaleAfter = c.StaleAfter
	}
	if c.Retention > 0 {
		d.Retention = c.Retention
	}
	if c.MaxAttempts > 0 {
		d.MaxAttempts = c.MaxAttempts
	}
	if c.RetryDelay > 0 {
		d.RetryDelay = c.RetryDelay
	}
	return d
}

// RunWriteWorker drains queued writes until ctx is cancelled. One job at a time,
// oldest first, and never two in the same namespace: that is what keeps supersede
// chains and keyed writes in the order they were asked for.
//
// It is safe to run this in more than one process — the claim statement skips
// locked rows and namespaces already in flight — which is what makes it usable in
// both the API and the MCP container.
func (b *Brain) RunWriteWorker(ctx context.Context, cfg WriteWorkerConfig) {
	cfg = cfg.withDefaults()

	if n, err := b.RecoverStaleWrites(ctx, cfg.StaleAfter); err != nil {
		log.Printf("write worker: recover stalled jobs: %v", err)
	} else if n > 0 {
		log.Printf("write worker: recovered %d stalled job(s)", n)
	}
	if n, err := b.PurgeFinishedWrites(ctx, cfg.Retention); err != nil {
		log.Printf("write worker: purge finished jobs: %v", err)
	} else if n > 0 {
		log.Printf("write worker: pruned %d finished job(s)", n)
	}

	poll := time.NewTicker(cfg.Poll)
	defer poll.Stop()
	housekeeping := time.NewTicker(time.Hour)
	defer housekeeping.Stop()

	log.Printf("write worker: draining write_jobs every %s", cfg.Poll)

	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
		case <-b.writeWake:
		case <-housekeeping.C:
			if n, err := b.RecoverStaleWrites(ctx, cfg.StaleAfter); err != nil {
				log.Printf("write worker: recover stalled jobs: %v", err)
			} else if n > 0 {
				log.Printf("write worker: recovered %d stalled job(s)", n)
			}
			if n, err := b.PurgeFinishedWrites(ctx, cfg.Retention); err != nil {
				log.Printf("write worker: purge finished jobs: %v", err)
			} else if n > 0 {
				log.Printf("write worker: pruned %d finished job(s)", n)
			}
		}
		b.drainWriteJobs(ctx, cfg)
	}
}

// drainWriteJobs empties the queue until it is empty or a claim fails.
func (b *Brain) drainWriteJobs(ctx context.Context, cfg WriteWorkerConfig) {
	for ctx.Err() == nil {
		job, payload, ok, err := b.ClaimWriteJob(ctx)
		if err != nil {
			log.Printf("write worker: claim: %v", err)
			return
		}
		if !ok {
			return
		}
		b.runWriteJob(ctx, *job, payload, cfg)
	}
}

// runWriteJob executes one claimed job and records its outcome. The write path is
// the same one a synchronous caller runs; nothing here is a shortcut.
func (b *Brain) runWriteJob(ctx context.Context, job models.WriteJob, payload WritePayload, cfg WriteWorkerConfig) {
	// The worker has no principal of its own. The actor that asked is restored so
	// provenance, the goal log and fact feedback still name the writer.
	runCtx := WithActor(ctx, job.Actor)

	out, err := b.RememberWithOutcome(runCtx, payload.Namespace, payload.Content, RememberOptions{
		OccurredAt:   payload.OccurredAt,
		Importance:   payload.Importance,
		Tags:         payload.Tags,
		ExternalKey:  payload.ExternalKey,
		ExternalRefs: payload.ExternalRefs,
		Supersedes:   payload.Supersedes,
		CompleteGoal: payload.CompleteGoal,
	})
	switch {
	case err == nil:
		if err := b.FinishWriteJob(ctx, job.ID, WriteJobDone, out.ID, nil); err != nil {
			log.Printf("write worker: job %d stored but not marked done: %v", job.ID, err)
		}
	case errors.Is(err, ErrSocialCloser), errors.Is(err, ErrWriteFiltered):
		// A refusal is an answer, not a failure: the caller would have received
		// the same verdict synchronously.
		if err := b.FinishWriteJob(ctx, job.ID, WriteJobSkipped, 0, err); err != nil {
			log.Printf("write worker: job %d skipped but not recorded: %v", job.ID, err)
		}
	case permanentWriteError(err):
		if err := b.FinishWriteJob(ctx, job.ID, WriteJobFailed, 0, err); err != nil {
			log.Printf("write worker: job %d failed but not recorded: %v", job.ID, err)
		}
		log.Printf("write worker: job %d failed: %v", job.ID, err)
	case job.Attempts < cfg.MaxAttempts:
		if err := b.RetryWriteJob(ctx, job.ID, err, time.Duration(job.Attempts)*cfg.RetryDelay); err != nil {
			log.Printf("write worker: job %d could not be retried: %v", job.ID, err)
		}
		log.Printf("write worker: job %d attempt %d failed, retrying: %v", job.ID, job.Attempts, err)
	default:
		if err := b.FinishWriteJob(ctx, job.ID, WriteJobFailed, 0, err); err != nil {
			log.Printf("write worker: job %d failed but not recorded: %v", job.ID, err)
		}
		log.Printf("write worker: job %d gave up after %d attempts: %v", job.ID, job.Attempts, err)
	}
}

// permanentWriteError says whether retrying could ever help. A rejected write, a
// taken key or a missing goal is a decision, not a hiccup; an embedding call that
// timed out is worth another try.
func permanentWriteError(err error) bool {
	return errors.Is(err, ErrEmptyContent) ||
		errors.Is(err, ErrNamespaceNotFound) ||
		errors.Is(err, ErrExternalKeyExists) ||
		errors.Is(err, ErrSupersedesNeedsKey) ||
		errors.Is(err, ErrGoalNotFound) ||
		errors.Is(err, ErrGoalNotActive) ||
		errors.Is(err, ErrReadOnly) ||
		errors.Is(err, ErrSocialCloser) ||
		errors.Is(err, ErrWriteFiltered)
}
