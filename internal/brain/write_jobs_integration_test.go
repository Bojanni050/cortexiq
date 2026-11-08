// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"errors"
	"testing"
	"time"
)

// A job that was accepted has to come out as a memory: that is the whole point of
// the queue. The episode id is the evidence the caller can point at afterwards.
func TestIntegrationAsyncWriteLandsWithItsEpisode(t *testing.T) {
	e := newEnv(t, nil)

	payload, _, err := e.b.AdmitWrite(e.ctx, e.ns, "the deferred write is stored by the worker", 0.5, []string{"Async"})
	if err != nil {
		t.Fatalf("AdmitWrite: %v", err)
	}
	if len(payload.Tags) != 1 || payload.Tags[0] != "async" {
		t.Errorf("payload tags = %v, want the normalized [async]", payload.Tags)
	}

	jobID, err := e.b.EnqueueWrite(e.ctx, payload)
	if err != nil {
		t.Fatalf("EnqueueWrite: %v", err)
	}
	if queued, _, err := e.b.WriteQueueDepth(e.ctx); err != nil || queued != 1 {
		t.Fatalf("queue depth = %d (err %v), want 1 queued", queued, err)
	}

	e.b.drainWriteJobs(e.ctx, DefaultWriteWorkerConfig())

	job, err := e.b.GetWriteJob(e.ctx, jobID)
	if err != nil {
		t.Fatalf("GetWriteJob: %v", err)
	}
	if job.State != WriteJobDone {
		t.Fatalf("job state = %q (%v), want done", job.State, job.Error)
	}
	if job.EpisodeID == nil {
		t.Fatal("a done job has no episode id, so the caller can never find its memory")
	}
	if job.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", job.Attempts)
	}
	if job.Actor == "" {
		t.Error("job has no actor")
	}

	var content string
	if err := e.b.pool.QueryRow(e.ctx, `SELECT content FROM episodes WHERE id = $1`, *job.EpisodeID).Scan(&content); err != nil {
		t.Fatalf("read episode %d: %v", *job.EpisodeID, err)
	}
	if content != payload.Content {
		t.Errorf("stored content = %q, want %q", content, payload.Content)
	}

	if queued, running, err := e.b.WriteQueueDepth(e.ctx); err != nil || queued != 0 || running != 0 {
		t.Errorf("queue after drain = %d queued, %d running (err %v)", queued, running, err)
	}
}

// A refusal at run time is an answer, not an error: the caller would have been
// told the same thing synchronously. This simulates a rule that changed between
// enqueue and run, which is exactly why the worker re-checks.
func TestIntegrationAsyncWriteRecordsARefusalAsSkipped(t *testing.T) {
	e := newEnv(t, nil)

	jobID, err := e.b.EnqueueWrite(e.ctx, WritePayload{
		Namespace:  e.ns,
		Content:    "dank je wel",
		Importance: DefaultImportance,
	})
	if err != nil {
		t.Fatalf("EnqueueWrite: %v", err)
	}

	e.b.drainWriteJobs(e.ctx, DefaultWriteWorkerConfig())

	job, err := e.b.GetWriteJob(e.ctx, jobID)
	if err != nil {
		t.Fatalf("GetWriteJob: %v", err)
	}
	if job.State != WriteJobSkipped {
		t.Errorf("job state = %q, want skipped", job.State)
	}
	if job.EpisodeID != nil {
		t.Errorf("a skipped write created episode %d", *job.EpisodeID)
	}
	if job.Error == nil {
		t.Error("a skipped job must say why it was skipped")
	}
}

// A permanent failure is final: retrying a taken key would fail forever.
func TestIntegrationAsyncWriteFailsOnATakenKey(t *testing.T) {
	e := newEnv(t, nil)

	if _, err := e.b.RememberWithOutcome(e.ctx, e.ns, "the first version of the statement", RememberOptions{
		ExternalKey: "gaia:hypothesis:1",
	}); err != nil {
		t.Fatalf("seed keyed write: %v", err)
	}

	jobID, err := e.b.EnqueueWrite(e.ctx, WritePayload{
		Namespace:   e.ns,
		Content:     "a different version of the statement",
		Importance:  DefaultImportance,
		ExternalKey: "gaia:hypothesis:1",
	})
	if err != nil {
		t.Fatalf("EnqueueWrite: %v", err)
	}

	e.b.drainWriteJobs(e.ctx, DefaultWriteWorkerConfig())

	job, err := e.b.GetWriteJob(e.ctx, jobID)
	if err != nil {
		t.Fatalf("GetWriteJob: %v", err)
	}
	if job.State != WriteJobFailed {
		t.Fatalf("job state = %q, want failed", job.State)
	}
	if job.Error == nil || *job.Error == "" {
		t.Error("a failed job must carry its reason")
	}
	// A permanent failure must not be requeued for another attempt.
	if queued, _, _ := e.b.WriteQueueDepth(e.ctx); queued != 0 {
		t.Errorf("a permanent failure was requeued: %d queued", queued)
	}
}

// A worker that dies mid-run leaves a running row behind. That row must not block
// its namespace forever.
func TestIntegrationStalledWriteIsRecovered(t *testing.T) {
	e := newEnv(t, nil)

	jobID, err := e.b.EnqueueWrite(e.ctx, WritePayload{
		Namespace:  e.ns,
		Content:    "a write whose worker died",
		Importance: DefaultImportance,
	})
	if err != nil {
		t.Fatalf("EnqueueWrite: %v", err)
	}
	if _, err := e.b.pool.Exec(e.ctx,
		`UPDATE write_jobs SET state = $2, attempts = 1, started_at = now() - interval '1 hour' WHERE id = $1`,
		jobID, WriteJobRunning,
	); err != nil {
		t.Fatalf("simulate a dead worker: %v", err)
	}

	n, err := e.b.RecoverStaleWrites(e.ctx, 10*time.Minute)
	if err != nil {
		t.Fatalf("RecoverStaleWrites: %v", err)
	}
	if n != 1 {
		t.Errorf("recovered %d jobs, want 1", n)
	}

	job, err := e.b.GetWriteJob(e.ctx, jobID)
	if err != nil {
		t.Fatalf("GetWriteJob: %v", err)
	}
	if job.State != WriteJobQueued {
		t.Errorf("job state after recovery = %q, want queued", job.State)
	}

	// And it runs on the next pass.
	e.b.drainWriteJobs(e.ctx, DefaultWriteWorkerConfig())
	job, err = e.b.GetWriteJob(e.ctx, jobID)
	if err != nil {
		t.Fatalf("GetWriteJob after drain: %v", err)
	}
	if job.State != WriteJobDone {
		t.Errorf("recovered job state = %q (%v), want done", job.State, job.Error)
	}
}

func TestPermanentWriteError(t *testing.T) {
	for _, err := range []error{ErrExternalKeyExists, ErrNamespaceNotFound, ErrGoalNotActive, ErrSocialCloser, ErrWriteFiltered} {
		if !permanentWriteError(err) {
			t.Errorf("%v should be permanent", err)
		}
	}
	// A transient failure has to be worth another attempt.
	if permanentWriteError(errors.New("dial tcp 127.0.0.1:11434: connect: connection refused")) {
		t.Error("a dead embedder must be retried, not declared permanent")
	}
}
