// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/jackc/pgx/v5"
)

// ErrWriteJobNotFound is what GetWriteJob answers for an unknown id.
var ErrWriteJobNotFound = errors.New("brain: write job not found")

// States a deferred write can be in. skipped is not an error: it is the refusal
// the gates would have given the caller immediately (noise, a social closer).
const (
	WriteJobQueued  = "queued"
	WriteJobRunning = "running"
	WriteJobDone    = "done"
	WriteJobSkipped = "skipped"
	WriteJobFailed  = "failed"
)

// WritePayload is a remember call stored for later. It is the caller's request,
// already through the noise filter and secret redaction, so the worker runs
// exactly what was meant and a restart needs nothing from the client.
type WritePayload struct {
	Namespace    string     `json:"namespace"`
	Content      string     `json:"content"`
	Importance   float32    `json:"importance,omitempty"`
	OccurredAt   *time.Time `json:"occurred_at,omitempty"`
	Tags         []string   `json:"tags,omitempty"`
	ExternalKey  string     `json:"external_key,omitempty"`
	ExternalRefs []string   `json:"external_refs,omitempty"`
	Supersedes   bool       `json:"supersedes,omitempty"`
	CompleteGoal *int64     `json:"complete_goal,omitempty"`
}

// AdmitWrite runs the checks that decide whether a write is worth queueing at
// all, without doing the write. A request that can never become a memory must be
// refused now: answering 202 for something the gates would reject turns a clear
// error into a failed job nobody watches.
//
// This is an early refusal, not the gate. The worker runs the full write path
// afterwards, so a rule that changes between enqueue and run still applies — and
// the redacted form is what gets stored, never the secret.
func (b *Brain) AdmitWrite(ctx context.Context, namespaceSlug, content string, importance float32, tags []string) (WritePayload, WriteDecision, error) {
	var payload WritePayload
	if err := requireWrite(ctx); err != nil {
		return payload, WriteDecision{}, err
	}
	if err := validateContent(content); err != nil {
		return payload, WriteDecision{}, err
	}
	if importance < 0 || importance > 1 || importance != importance {
		return payload, WriteDecision{}, fmt.Errorf("importance must be between 0 and 1, got %v", importance)
	}
	normTags, err := NormalizeTags(tags)
	if err != nil {
		return payload, WriteDecision{}, err
	}
	if err := validatePath(namespaceSlug); err != nil {
		return payload, WriteDecision{}, err
	}
	if _, err := b.resolveNamespaceID(ctx, namespaceSlug); err != nil {
		return payload, WriteDecision{}, err
	}

	decision := classifyWrite(content, importance, b.config.WriteFilter)
	if err := applyWriteDecision(decision); err != nil {
		return payload, decision, err
	}
	if decision.Action == "warn" && decision.Redacted != "" && b.config.WriteFilterRedactSecrets {
		content = decision.Redacted
	}

	socialFilter := b.config.SocialFilter
	if eff, err := b.EffectiveNamespaceConfig(ctx, namespaceSlug); err == nil && eff != nil {
		socialFilter = eff.SocialFilter()
	}
	if socialFilter && socialCloser(content) {
		return payload, decision, ErrSocialCloser
	}

	payload = WritePayload{
		Namespace:  namespaceSlug,
		Content:    content,
		Importance: importance,
		Tags:       normTags,
	}
	return payload, decision, nil
}

// EnqueueWrite stores the payload as a job. From the moment this returns, the
// write survives a crash of either process, which is why the caller may report
// "accepted" instead of "stored".
func (b *Brain) EnqueueWrite(ctx context.Context, payload WritePayload) (int64, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshal write payload: %w", err)
	}
	nsID, err := b.resolveNamespaceID(ctx, payload.Namespace)
	if err != nil {
		return 0, err
	}

	var id int64
	err = b.pool.QueryRow(ctx,
		`INSERT INTO write_jobs (namespace_id, namespace_slug, actor, payload)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		nsID, payload.Namespace, actorFrom(ctx), raw,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("enqueue write: %w", err)
	}
	// Wake this process's worker right away; a queue that waits for the next poll
	// makes an accepted write feel lost.
	select {
	case b.writeWake <- struct{}{}:
	default:
	}
	return id, nil
}

const writeJobColumns = `id, namespace_id, namespace_slug, actor, state, attempts,
episode_id, error, created_at, started_at, finished_at`

func scanWriteJob(row pgx.Row, withPayload bool) (*models.WriteJob, error) {
	var j models.WriteJob
	dest := []any{&j.ID, &j.NamespaceID, &j.Namespace, &j.Actor, &j.State, &j.Attempts,
		&j.EpisodeID, &j.Error, &j.CreatedAt, &j.StartedAt, &j.FinishedAt}
	if withPayload {
		dest = append(dest, &j.Payload)
	}
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return &j, nil
}

// ClaimWriteJob takes the oldest runnable job and marks it running in one
// statement, so two workers can never pick the same row. A namespace that already
// has a job running is skipped: writes to one namespace must land in the order
// they were asked for, or a supersede chain applies to the wrong version.
func (b *Brain) ClaimWriteJob(ctx context.Context) (*models.WriteJob, WritePayload, bool, error) {
	job, err := scanWriteJob(b.pool.QueryRow(ctx,
		`UPDATE write_jobs SET state = $1, attempts = attempts + 1, started_at = now()
		 WHERE id = (
		   SELECT j.id FROM write_jobs j
		   WHERE j.state = $2
		     AND (j.next_attempt_at IS NULL OR j.next_attempt_at <= now())
		     AND NOT EXISTS (
		       SELECT 1 FROM write_jobs r WHERE r.state = $1 AND r.namespace_id = j.namespace_id
		     )
		   ORDER BY j.id
		   LIMIT 1
		   FOR UPDATE SKIP LOCKED
		 )
		 RETURNING `+writeJobColumns+`, payload`,
		WriteJobRunning, WriteJobQueued,
	), true)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, WritePayload{}, false, nil
	}
	if err != nil {
		return nil, WritePayload{}, false, fmt.Errorf("claim write job: %w", err)
	}

	var payload WritePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		// A job whose payload cannot be read can never run; record why instead of
		// failing on every pass.
		finishErr := b.FinishWriteJob(ctx, job.ID, WriteJobFailed, 0, fmt.Errorf("unreadable payload: %w", err))
		if finishErr != nil {
			return nil, WritePayload{}, false, finishErr
		}
		return nil, WritePayload{}, false, nil
	}
	return job, payload, true, nil
}

// FinishWriteJob records the final state. episodeID is 0 when nothing was
// written (a skip or a failure).
func (b *Brain) FinishWriteJob(ctx context.Context, id int64, state string, episodeID int64, cause error) error {
	var episode *int64
	if episodeID > 0 {
		episode = &episodeID
	}
	var reason *string
	if cause != nil {
		s := cause.Error()
		reason = &s
	}
	_, err := b.pool.Exec(ctx,
		`UPDATE write_jobs SET state = $2, episode_id = $3, error = $4, finished_at = now()
		 WHERE id = $1`, id, state, episode, reason)
	if err != nil {
		return fmt.Errorf("finish write job %d: %w", id, err)
	}
	return nil
}

// RetryWriteJob puts a failed attempt back in the queue after delay, so a broken
// dependency cannot make the worker spin.
func (b *Brain) RetryWriteJob(ctx context.Context, id int64, cause error, delay time.Duration) error {
	reason := cause.Error()
	_, err := b.pool.Exec(ctx,
		`UPDATE write_jobs
		 SET state = $2, error = $3, started_at = NULL, finished_at = NULL,
		     next_attempt_at = now() + make_interval(secs => $4)
		 WHERE id = $1`, id, WriteJobQueued, reason, delay.Seconds())
	if err != nil {
		return fmt.Errorf("retry write job %d: %w", id, err)
	}
	return nil
}

// RecoverStaleWrites requeues jobs whose worker died mid-run. staleAfter is
// generous on purpose: a cold embedding model takes seconds, not minutes, so a
// job stuck past that is a dead worker rather than a slow one.
func (b *Brain) RecoverStaleWrites(ctx context.Context, staleAfter time.Duration) (int64, error) {
	tag, err := b.pool.Exec(ctx,
		`UPDATE write_jobs
		 SET state = $1, started_at = NULL, next_attempt_at = NULL,
		     error = 'recovered after a stalled worker'
		 WHERE state = $2 AND started_at < now() - make_interval(secs => $3)`,
		WriteJobQueued, WriteJobRunning, staleAfter.Seconds())
	if err != nil {
		return 0, fmt.Errorf("recover stale writes: %w", err)
	}
	return tag.RowsAffected(), nil
}

// PurgeFinishedWrites drops finished jobs past their retention window. The queue
// must not grow without bound: a done job has nothing left to promise.
func (b *Brain) PurgeFinishedWrites(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := b.pool.Exec(ctx,
		`DELETE FROM write_jobs
		 WHERE state IN ($1, $2, $3) AND finished_at IS NOT NULL
		   AND finished_at < now() - make_interval(secs => $4)`,
		WriteJobDone, WriteJobSkipped, WriteJobFailed, olderThan.Seconds())
	if err != nil {
		return 0, fmt.Errorf("purge finished writes: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListWriteJobs returns jobs newest first, optionally filtered by state. The
// payload is left out: a queue overview is about what is pending, not about the
// text of every row.
func (b *Brain) ListWriteJobs(ctx context.Context, state string, page Pagination) ([]models.WriteJob, error) {
	page = page.Sanitize()
	args := []any{}
	where := ""
	if state != "" {
		args = append(args, state)
		where = " WHERE state = $1"
	}
	query := `SELECT ` + writeJobColumns + ` FROM write_jobs` + where +
		fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, page.Limit, page.Offset)

	rows, err := b.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list write jobs: %w", err)
	}
	defer rows.Close()

	var result []models.WriteJob
	for rows.Next() {
		var j models.WriteJob
		if err := rows.Scan(&j.ID, &j.NamespaceID, &j.Namespace, &j.Actor, &j.State, &j.Attempts,
			&j.EpisodeID, &j.Error, &j.CreatedAt, &j.StartedAt, &j.FinishedAt); err != nil {
			return nil, fmt.Errorf("scan write job: %w", err)
		}
		result = append(result, j)
	}
	return result, rows.Err()
}

// CountWriteJobs counts what ListWriteJobs would return for the same filter.
func (b *Brain) CountWriteJobs(ctx context.Context, state string) (int, error) {
	if state == "" {
		return b.countWhere(ctx, "write_jobs", "TRUE")
	}
	return b.countWhere(ctx, "write_jobs", "state = $1", state)
}

// GetWriteJob returns one job including its payload.
func (b *Brain) GetWriteJob(ctx context.Context, id int64) (*models.WriteJob, error) {
	job, err := scanWriteJob(b.pool.QueryRow(ctx,
		`SELECT `+writeJobColumns+`, payload FROM write_jobs WHERE id = $1`, id), true)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWriteJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get write job: %w", err)
	}
	return job, nil
}

// WriteQueueDepth reports how much work is waiting and how much is in flight,
// which is what a health check and the dashboard need.
func (b *Brain) WriteQueueDepth(ctx context.Context) (queued, running int, err error) {
	err = b.pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE state = $1), COUNT(*) FILTER (WHERE state = $2)
		 FROM write_jobs`, WriteJobQueued, WriteJobRunning).Scan(&queued, &running)
	if err != nil {
		return 0, 0, fmt.Errorf("write queue depth: %w", err)
	}
	return queued, running, nil
}
