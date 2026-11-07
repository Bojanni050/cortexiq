// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/Bojanni050/cortexiq/internal/reasoner"
	"github.com/jackc/pgx/v5"
)

// ActorCritic is the actor for a belief the critic model proposed. It is
// separate from ActorConsolidator because a counter is not evidence: it is an
// argument, and the log should not blur the two.
const ActorCritic = "critic"

// counterHypothesisTimeout bounds the critic call. A hypothesis that already
// exists must not hang a request because a second model is slow, and the
// reasoner's own timeout is generous enough that a stuck call here means the
// provider is unhealthy rather than the prompt being hard.
const counterHypothesisTimeout = 45 * time.Second

// maxCounterEvidence caps the facts handed to the critic. Beyond a handful the
// prompt stops being about the hypothesis and starts being about the corpus.
const maxCounterEvidence = 10

// proposeCounterHypothesis asks the critic model for the strongest opposing
// belief and stores it linked to the original.
//
// It is best-effort on purpose: the hypothesis was already created and is
// valid, so a critic that is unreachable, refuses, or answers garbage must not
// turn a successful write into an error. The failure is logged instead, which
// is the same honesty the hypothesis log applies everywhere else.
func (b *Brain) proposeCounterHypothesis(ctx context.Context, h *models.Hypothesis) {
	if h.CounterOf != nil {
		// A counter is an argument against a belief, not a belief of its own.
		// Letting it spawn a counter of a counter produces an infinite argument
		// that says nothing about the world.
		return
	}
	critic := b.critic
	model := b.criticModel
	independent := critic != nil
	if critic == nil {
		critic = b.reasoner
		model = ""
	}
	if critic == nil {
		return
	}

	var exists bool
	if err := b.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM hypotheses WHERE counter_of = $1 AND deleted_at IS NULL)`, h.ID,
	).Scan(&exists); err != nil {
		log.Printf("counter hypothesis: cannot check for an existing counter of #%d: %v", h.ID, err)
		return
	}
	if exists {
		return
	}

	evidence, err := b.FactsByIDs(ctx, h.SourceFactIDs)
	if err != nil {
		// The critic can still argue from the hypothesis itself; the facts are
		// context, not a requirement.
		log.Printf("counter hypothesis: cannot load source facts for #%d: %v", h.ID, err)
	}
	if len(evidence) > maxCounterEvidence {
		evidence = evidence[:maxCounterEvidence]
	}

	callCtx, cancel := context.WithTimeout(ctx, counterHypothesisTimeout)
	defer cancel()

	start := time.Now()
	counter, err := critic.ProposeCounterHypothesis(callCtx, *h, evidence)
	took := time.Since(start).Round(time.Millisecond)
	if err != nil {
		log.Printf("counter hypothesis: hypothesis=#%d independent=%t took=%s error=%q", h.ID, independent, took, err.Error())
		return
	}

	created, err := b.insertCounterHypothesis(ctx, h, counter, independent, model)
	if err != nil {
		log.Printf("counter hypothesis: hypothesis=#%d took=%s store failed: %v", h.ID, took, err)
		return
	}

	log.Printf("counter hypothesis: hypothesis=#%d counter=#%d independent=%t model=%q took=%s",
		h.ID, created.ID, independent, model, took)
}

// insertCounterHypothesis writes the counter row and the two log entries: the
// counter's own proposal, and a `counter` event on the original so its history
// shows that someone argued the other side.
func (b *Brain) insertCounterHypothesis(ctx context.Context, h *models.Hypothesis, c *reasoner.CounterHypothesis, independent bool, model string) (*models.Hypothesis, error) {
	sourceIDs := nonNilIDs(h.SourceFactIDs)

	var out models.Hypothesis
	err := b.pool.QueryRow(ctx,
		`INSERT INTO hypotheses (namespace_id, content, confidence, verification_plan, source_fact_ids, supported_by, method, counter_of)
		 VALUES ($1, $2, $3, $4, $5, $5, 'counter', $6)
		 RETURNING id, namespace_id, content, confidence, status, verification_plan, method,
		 confirmed_fact_id, rejection_reason, source_fact_ids, counter_of, tested_at, confirmed_at, rejected_at,
		 created_at, updated_at, deleted_at`,
		h.NamespaceID, c.Content, c.Confidence, c.VerificationPlan, sourceIDs, h.ID,
	).Scan(
		&out.ID, &out.NamespaceID, &out.Content, &out.Confidence, &out.Status,
		&out.VerificationPlan, &out.Method, &out.ConfirmedFactID, &out.RejectionReason,
		&out.SourceFactIDs, &out.CounterOf, &out.TestedAt, &out.ConfirmedAt, &out.RejectedAt,
		&out.CreatedAt, &out.UpdatedAt, &out.DeletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert counter hypothesis: %w", err)
	}
	// The counter stands on the same evidence as the belief it opposes; it is
	// a different reading of those facts, not a new claim about the world.
	out.SupportedBy = sourceIDs

	// The counter is written as the critic's proposal, not as the caller's: the
	// human who created the original did not assert this.
	criticCtx := WithActor(ctx, ActorCritic)
	if err := recordHypothesisEvent(criticCtx, b.pool, hypothesisEvent{
		HypothesisID: out.ID,
		NamespaceID:  out.NamespaceID,
		Event:        HypothesisEventProposed,
		StatusTo:     out.Status,
		ConfidenceTo: &out.Confidence,
		Detail: map[string]any{
			"method":       out.Method,
			"counter_of":   h.ID,
			"independent":  independent,
			"critic_model": model,
			"reasoning":    c.Reasoning,
		},
	}); err != nil {
		return nil, err
	}

	status := h.Status
	if err := recordHypothesisEvent(ctx, b.pool, hypothesisEvent{
		HypothesisID: h.ID,
		NamespaceID:  h.NamespaceID,
		Event:        HypothesisEventCounter,
		StatusFrom:   &status,
		StatusTo:     status,
		Detail: map[string]any{
			"counter_id":  out.ID,
			"independent": independent,
			"reasoning":   c.Reasoning,
		},
	}); err != nil {
		return nil, err
	}

	return &out, nil
}

// CounterHypothesisFor returns the counter that argues against the given
// hypothesis, or nil when none was proposed.
func (b *Brain) CounterHypothesisFor(ctx context.Context, hypothesisID int64) (*models.Hypothesis, error) {
	var h models.Hypothesis
	err := b.pool.QueryRow(ctx,
		`SELECT id, namespace_id, content, confidence, status, verification_plan, method,
		 confirmed_fact_id, rejection_reason, source_fact_ids, counter_of, tested_at, confirmed_at, rejected_at,
		 created_at, updated_at, deleted_at
		 FROM hypotheses WHERE counter_of = $1 AND deleted_at IS NULL`,
		hypothesisID,
	).Scan(
		&h.ID, &h.NamespaceID, &h.Content, &h.Confidence, &h.Status,
		&h.VerificationPlan, &h.Method, &h.ConfirmedFactID, &h.RejectionReason,
		&h.SourceFactIDs, &h.CounterOf, &h.TestedAt, &h.ConfirmedAt, &h.RejectedAt,
		&h.CreatedAt, &h.UpdatedAt, &h.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("counter hypothesis for %d: %w", hypothesisID, err)
	}
	return &h, nil
}

// counterpartIsConfirmed reports whether the hypothesis on the other side of the
// pair is already a fact. The original and its counter are two answers to one
// question, so only one of them may become a fact without a human deciding.
func (b *Brain) counterpartIsConfirmed(ctx context.Context, h *models.Hypothesis) (bool, error) {
	var counterpart int64
	if h.CounterOf != nil {
		counterpart = *h.CounterOf
	} else if err := b.pool.QueryRow(ctx,
		`SELECT id FROM hypotheses WHERE counter_of = $1 AND deleted_at IS NULL`, h.ID,
	).Scan(&counterpart); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("find counter of %d: %w", h.ID, err)
	}

	var confirmed bool
	err := b.pool.QueryRow(ctx,
		`SELECT status = 'confirmed' FROM hypotheses WHERE id = $1 AND deleted_at IS NULL`, counterpart,
	).Scan(&confirmed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("check counterpart %d: %w", counterpart, err)
	}
	return confirmed, nil
}
