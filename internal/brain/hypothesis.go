// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/jackc/pgx/v5"
)

var (
	ErrHypothesisNotFound          = fmt.Errorf("brain: hypothesis not found")
	ErrInvalidHypothesisTransition = fmt.Errorf("brain: invalid hypothesis status transition")
	ErrHypothesisAlreadyConfirmed  = fmt.Errorf("brain: hypothesis already confirmed")
)

var validTransitions = map[string][]string{
	"proposed":  {"testing", "rejected"},
	"testing":   {"confirmed", "rejected", "proposed"},
	"confirmed": {},
	"rejected":  {},
}

func isValidTransition(from, to string) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

func scanHypothesis(h *models.Hypothesis, row pgx.Row) error {
	return row.Scan(
		&h.ID, &h.NamespaceID, &h.Content, &h.Confidence, &h.Status,
		&h.VerificationPlan, &h.Method, &h.ConfirmedFactID, &h.RejectionReason,
		&h.SourceFactIDs, &h.TestedAt, &h.ConfirmedAt, &h.RejectedAt,
		&h.CreatedAt, &h.UpdatedAt, &h.DeletedAt, &h.CounterOf,
	)
}

// scanHypothesisRows maps rows by column name, not by position. Positional
// scanning broke twice on the same column: counter_of was added to the struct
// and forgotten in two SELECTs, which showed up as an empty dashboard tab and,
// worse, as a consolidator that silently skipped every open hypothesis. By
// name, a forgotten column is a named error instead of a silent mismatch.
func scanHypothesisRows(rows pgx.Rows) ([]models.Hypothesis, error) {
	result, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Hypothesis])
	if err != nil {
		return nil, fmt.Errorf("scan hypothesis: %w", err)
	}
	return result, nil
}

// hypothesisInput is everything an insert needs. Method is explicit rather than
// left to the column default because a belief the consolidator inferred itself
// must stay distinguishable from one an agent asserted — the dashboard and the
// auto-confirm guard both depend on that difference.
type hypothesisInput struct {
	NamespaceID      int64
	Content          string
	VerificationPlan string
	Confidence       float32
	SourceFactIDs    []int64
	Method           string
	// Detail is merged into the `proposed` log entry (reasoning, provenance).
	Detail map[string]any
}

// ErrUnknownSourceFacts means a caller cited evidence that is not in memory.
// It is refused rather than quietly emptied: a belief that names its evidence
// and then has none is worse than one that never named any.
var ErrUnknownSourceFacts = fmt.Errorf("brain: hypothesis: source facts do not exist")

// verifiedFactIDs keeps only the ids that actually exist, in the order given.
// Every path that creates a hypothesis goes through here, because the ids can
// come from a model, an agent or a human, and none of them is trusted.
func (b *Brain) verifiedFactIDs(ctx context.Context, ids []int64) ([]int64, error) {
	ids = nonNilIDs(ids)
	if len(ids) == 0 {
		// Not nil: source_fact_ids is NOT NULL, and a nil slice becomes NULL.
		return []int64{}, nil
	}

	rows, err := b.pool.Query(ctx, `SELECT id FROM facts WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("verify source facts: %w", err)
	}
	defer rows.Close()

	known := make(map[int64]bool, len(ids))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan source fact id: %w", err)
		}
		known[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(known) == 0 {
		return nil, fmt.Errorf("%w: %v", ErrUnknownSourceFacts, ids)
	}

	kept := make([]int64, 0, len(ids))
	for _, id := range ids {
		if known[id] {
			kept = append(kept, id)
			delete(known, id)
		}
	}
	return kept, nil
}

func (b *Brain) insertHypothesis(ctx context.Context, in hypothesisInput) (*models.Hypothesis, error) {
	if in.Content == "" {
		return nil, ErrEmptyContent
	}
	sourceFactIDs, err := b.verifiedFactIDs(ctx, in.SourceFactIDs)
	if err != nil {
		return nil, err
	}
	method := in.Method
	if method == "" {
		method = "asserted"
	}

	var h models.Hypothesis
	err = b.pool.QueryRow(ctx,
		`INSERT INTO hypotheses (namespace_id, content, confidence, verification_plan, source_fact_ids, supported_by, method)
		 VALUES ($1, $2, $3, $4, $5, $5, $6)
		 RETURNING id, namespace_id, content, confidence, status, verification_plan, method,
		 confirmed_fact_id, rejection_reason, source_fact_ids, tested_at, confirmed_at, rejected_at,
		 created_at, updated_at, deleted_at, counter_of`,
		in.NamespaceID, in.Content, in.Confidence, in.VerificationPlan, sourceFactIDs, method,
	).Scan(
		&h.ID, &h.NamespaceID, &h.Content, &h.Confidence, &h.Status,
		&h.VerificationPlan, &h.Method, &h.ConfirmedFactID, &h.RejectionReason,
		&h.SourceFactIDs, &h.TestedAt, &h.ConfirmedAt, &h.RejectedAt,
		&h.CreatedAt, &h.UpdatedAt, &h.DeletedAt, &h.CounterOf,
	)
	if err != nil {
		return nil, fmt.Errorf("create hypothesis: %w", err)
	}
	// The belief starts out resting on what it was built from.
	h.SupportedBy = sourceFactIDs

	detail := map[string]any{"method": h.Method, "source_fact_ids": sourceFactIDs}
	for k, v := range in.Detail {
		detail[k] = v
	}
	if err := recordHypothesisEvent(ctx, b.pool, hypothesisEvent{
		HypothesisID: h.ID,
		NamespaceID:  in.NamespaceID,
		Event:        HypothesisEventProposed,
		StatusTo:     h.Status,
		ConfidenceTo: &h.Confidence,
		Detail:       detail,
	}); err != nil {
		return nil, err
	}

	// A belief without an opposing case is only half a test. The counter is
	// best-effort and never fails the creation; see proposeCounterHypothesis.
	b.proposeCounterHypothesis(ctx, &h)

	return &h, nil
}

// CreateHypothesis creates a new hypothesis in proposed status.
func (b *Brain) CreateHypothesis(ctx context.Context, nsID int64, content, verificationPlan string, confidence float32, sourceFactIDs []int64) (*models.Hypothesis, error) {
	return b.insertHypothesis(ctx, hypothesisInput{
		NamespaceID:      nsID,
		Content:          content,
		VerificationPlan: verificationPlan,
		Confidence:       confidence,
		SourceFactIDs:    sourceFactIDs,
		Method:           "asserted",
	})
}

// hypothesisWhere builds the filter ListHypotheses and CountHypotheses share.
func hypothesisWhere(nsIDs []int64, status string) (string, []any) {
	where := "namespace_id = ANY($1) AND deleted_at IS NULL"
	args := []any{nsIDs}
	if status != "" {
		args = append(args, status)
		where += fmt.Sprintf(" AND status = $%d", len(args))
	}
	return where, args
}

// CountHypotheses counts the hypotheses ListHypotheses would return for the same
// filter, ignoring paging.
func (b *Brain) CountHypotheses(ctx context.Context, namespaceSlugs []string, status string) (int, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return 0, err
	}
	where, args := hypothesisWhere(nsIDs, status)
	return b.countWhere(ctx, "hypotheses", where, args...)
}

// ListHypotheses returns hypotheses across namespaces, optionally filtered by status.
func (b *Brain) ListHypotheses(ctx context.Context, namespaceSlugs []string, status string, page Pagination) ([]models.Hypothesis, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}

	page = page.Sanitize()

	where, args := hypothesisWhere(nsIDs, status)
	query := `SELECT id, namespace_id, content, confidence, status, verification_plan, method,
	          confirmed_fact_id, rejection_reason, source_fact_ids, tested_at, confirmed_at, rejected_at,
	          created_at, updated_at, deleted_at, counter_of, supported_by
	          FROM hypotheses WHERE ` + where +
		fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, page.Limit, page.Offset)

	rows, err := b.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list hypotheses: %w", err)
	}
	defer rows.Close()
	hyps, err := scanHypothesisRows(rows)
	if err != nil {
		return nil, err
	}
	if err := b.attachSourceFacts(ctx, hyps); err != nil {
		return nil, err
	}
	return hyps, nil
}

// attachSourceFacts resolves the evidence behind a page of hypotheses in one
// query. A fact that no longer exists keeps its id with empty content, so a
// reader sees that it is gone instead of watching the id disappear.
//
// The same rows are the proof count: supported_by is every fact that has ever
// argued for the belief. Episodes come from fact_sources, because six facts
// from one conversation are one leg, not six.
func (b *Brain) attachSourceFacts(ctx context.Context, hyps []models.Hypothesis) error {
	seen := make(map[int64]struct{})
	var ids []int64
	add := func(id int64) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, h := range hyps {
		for _, id := range h.SourceFactIDs {
			add(id)
		}
		for _, id := range h.SupportedBy {
			add(id)
		}
		if h.ConfirmedFactID != nil {
			add(*h.ConfirmedFactID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	rows, err := b.pool.Query(ctx, `SELECT id, content FROM facts WHERE id = ANY($1)`, ids)
	if err != nil {
		return fmt.Errorf("source facts: %w", err)
	}

	byID := make(map[int64]string, len(ids))
	for rows.Next() {
		var id int64
		var content string
		if err := rows.Scan(&id, &content); err != nil {
			rows.Close()
			return fmt.Errorf("scan source fact: %w", err)
		}
		byID[id] = content
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	episodes, err := b.factEpisodes(ctx, ids)
	if err != nil {
		return err
	}

	for i := range hyps {
		refs := make([]models.FactRef, 0, len(hyps[i].SourceFactIDs))
		for _, id := range hyps[i].SourceFactIDs {
			refs = append(refs, models.FactRef{ID: id, Content: byID[id]})
		}
		hyps[i].SourceFacts = refs

		hyps[i].SupportFacts = len(hyps[i].SupportedBy)
		distinct := make(map[int64]struct{})
		for _, id := range hyps[i].SupportedBy {
			for _, ep := range episodes[id] {
				distinct[ep] = struct{}{}
			}
		}
		hyps[i].SupportEpisodes = len(distinct)
	}
	return nil
}

// factEpisodes maps each fact to the episodes it was extracted from. A fact
// with no source row is simply absent from the map.
func (b *Brain) factEpisodes(ctx context.Context, ids []int64) (map[int64][]int64, error) {
	rows, err := b.pool.Query(ctx,
		`SELECT fact_id, episode_id FROM fact_sources WHERE fact_id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("fact sources: %w", err)
	}
	defer rows.Close()

	out := make(map[int64][]int64, len(ids))
	for rows.Next() {
		var factID, episodeID int64
		if err := rows.Scan(&factID, &episodeID); err != nil {
			return nil, fmt.Errorf("scan fact source: %w", err)
		}
		out[factID] = append(out[factID], episodeID)
	}
	return out, rows.Err()
}

// GetHypothesis returns a single hypothesis by ID.
func (b *Brain) GetHypothesis(ctx context.Context, id int64) (*models.Hypothesis, error) {
	var h models.Hypothesis
	err := b.pool.QueryRow(ctx,
		`SELECT id, namespace_id, content, confidence, status, verification_plan, method,
		 confirmed_fact_id, rejection_reason, source_fact_ids, tested_at, confirmed_at, rejected_at,
		 created_at, updated_at, deleted_at, counter_of
		 FROM hypotheses WHERE id = $1`,
		id,
	).Scan(
		&h.ID, &h.NamespaceID, &h.Content, &h.Confidence, &h.Status,
		&h.VerificationPlan, &h.Method, &h.ConfirmedFactID, &h.RejectionReason,
		&h.SourceFactIDs, &h.TestedAt, &h.ConfirmedAt, &h.RejectedAt,
		&h.CreatedAt, &h.UpdatedAt, &h.DeletedAt, &h.CounterOf,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrHypothesisNotFound
		}
		return nil, fmt.Errorf("get hypothesis: %w", err)
	}
	one := []models.Hypothesis{h}
	if err := b.attachSourceFacts(ctx, one); err != nil {
		return nil, err
	}
	h.SourceFacts = one[0].SourceFacts
	return &h, nil
}

// UpdateHypothesisStatus transitions a hypothesis to a new status.
func (b *Brain) UpdateHypothesisStatus(ctx context.Context, id int64, status string) (*models.Hypothesis, error) {
	current, err := b.GetHypothesis(ctx, id)
	if err != nil {
		return nil, err
	}

	if !isValidTransition(current.Status, status) {
		return nil, fmt.Errorf("%w: %s → %s", ErrInvalidHypothesisTransition, current.Status, status)
	}

	now := time.Now().UTC()
	var testedAt, confirmedAt, rejectedAt *time.Time
	if current.TestedAt != nil {
		testedAt = current.TestedAt
	}
	if current.ConfirmedAt != nil {
		confirmedAt = current.ConfirmedAt
	}
	if current.RejectedAt != nil {
		rejectedAt = current.RejectedAt
	}

	switch status {
	case "testing":
		testedAt = &now
	case "confirmed":
		confirmedAt = &now
	case "rejected":
		rejectedAt = &now
	}

	var h models.Hypothesis
	err = b.pool.QueryRow(ctx,
		`UPDATE hypotheses SET status = $2, tested_at = $3, confirmed_at = $4, rejected_at = $5, updated_at = $6
		 WHERE id = $1
		 RETURNING id, namespace_id, content, confidence, status, verification_plan, method,
		 confirmed_fact_id, rejection_reason, source_fact_ids, tested_at, confirmed_at, rejected_at,
		 created_at, updated_at, deleted_at, counter_of`,
		id, status, testedAt, confirmedAt, rejectedAt, now,
	).Scan(
		&h.ID, &h.NamespaceID, &h.Content, &h.Confidence, &h.Status,
		&h.VerificationPlan, &h.Method, &h.ConfirmedFactID, &h.RejectionReason,
		&h.SourceFactIDs, &h.TestedAt, &h.ConfirmedAt, &h.RejectedAt,
		&h.CreatedAt, &h.UpdatedAt, &h.DeletedAt, &h.CounterOf,
	)
	if err != nil {
		return nil, fmt.Errorf("update hypothesis status: %w", err)
	}
	if err := recordHypothesisEvent(ctx, b.pool, hypothesisEvent{
		HypothesisID: id,
		NamespaceID:  h.NamespaceID,
		Event:        status,
		StatusFrom:   strPtr(current.Status),
		StatusTo:     status,
	}); err != nil {
		return nil, err
	}
	return &h, nil
}

// ConfirmHypothesis confirms a hypothesis and auto-creates a Fact from its content.
func (b *Brain) ConfirmHypothesis(ctx context.Context, id int64) (*models.Hypothesis, *models.Fact, error) {
	current, err := b.GetHypothesis(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	if current.ConfirmedFactID != nil {
		return nil, nil, ErrHypothesisAlreadyConfirmed
	}

	if !isValidTransition(current.Status, "confirmed") {
		return nil, nil, fmt.Errorf("%w: %s → confirmed", ErrInvalidHypothesisTransition, current.Status)
	}

	vec, err := b.embedder.Embed(ctx, current.Content)
	if err != nil {
		return nil, nil, fmt.Errorf("embed confirmed hypothesis: %w", err)
	}

	now := time.Now().UTC()

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin confirm transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// The episodes behind the facts the hypothesis was drawn from are the
	// evidence trail. A hypothesis made from nothing has none, and then the
	// hypothesis itself is the reference.
	var sourceEpisodes []int64
	if len(current.SourceFactIDs) > 0 {
		rows, err := tx.Query(ctx,
			`SELECT DISTINCT episode_id FROM fact_sources WHERE fact_id = ANY($1)`, current.SourceFactIDs)
		if err != nil {
			return nil, nil, fmt.Errorf("read hypothesis evidence: %w", err)
		}
		for rows.Next() {
			var eid int64
			if err := rows.Scan(&eid); err != nil {
				rows.Close()
				return nil, nil, fmt.Errorf("scan hypothesis evidence: %w", err)
			}
			sourceEpisodes = append(sourceEpisodes, eid)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, nil, fmt.Errorf("read hypothesis evidence: %w", err)
		}
	}

	factID, err := b.insertFact(ctx, tx, factInsert{
		SourceEpisodeIDs: sourceEpisodes,
		OriginRef:        fmt.Sprintf("hypothesis:%d", id),
		Origin:           originHypothesis,
		NamespaceID:      current.NamespaceID,
		Content:          current.Content,
		Embedding:        vec,
		EmbeddingModel:   b.embedder.Model(),
		Confidence:       current.Confidence,
		ValidFrom:        &now,
		Importance:       DefaultImportance,
	})
	if err != nil {
		return nil, nil, err
	}

	var h models.Hypothesis
	err = tx.QueryRow(ctx,
		`UPDATE hypotheses SET status = 'confirmed', confirmed_at = $2, confirmed_fact_id = $3, updated_at = $2
		 WHERE id = $1
		 RETURNING id, namespace_id, content, confidence, status, verification_plan, method,
		 confirmed_fact_id, rejection_reason, source_fact_ids, tested_at, confirmed_at, rejected_at,
		 created_at, updated_at, deleted_at, counter_of`,
		id, now, factID,
	).Scan(
		&h.ID, &h.NamespaceID, &h.Content, &h.Confidence, &h.Status,
		&h.VerificationPlan, &h.Method, &h.ConfirmedFactID, &h.RejectionReason,
		&h.SourceFactIDs, &h.TestedAt, &h.ConfirmedAt, &h.RejectedAt,
		&h.CreatedAt, &h.UpdatedAt, &h.DeletedAt, &h.CounterOf,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("update hypothesis confirmed: %w", err)
	}

	// Inside the transaction: a confirmed hypothesis without the log entry that
	// says why would be the exact gap this table exists to close.
	if err := recordHypothesisEvent(ctx, tx, hypothesisEvent{
		HypothesisID:   id,
		NamespaceID:    h.NamespaceID,
		Event:          HypothesisEventConfirmed,
		StatusFrom:     strPtr(current.Status),
		StatusTo:       "confirmed",
		ConfidenceFrom: &current.Confidence,
		ConfidenceTo:   &h.Confidence,
		FactID:         &factID,
		Detail:         map[string]any{"source_episode_ids": sourceEpisodes, "source_fact_ids": current.SourceFactIDs},
	}); err != nil {
		return nil, nil, err
	}

	var f models.Fact
	err = tx.QueryRow(ctx,
		`SELECT id, namespace_id, content, embedding, embedding_model, confidence,
		 entity, property, value, valid_from, valid_until, created_at, updated_at, deleted_at, tags
		 FROM facts WHERE id = $1`,
		factID,
	).Scan(
		&f.ID, &f.NamespaceID, &f.Content, &f.Embedding, &f.EmbeddingModel,
		&f.Confidence, &f.Entity, &f.Property, &f.Value,
		&f.ValidFrom, &f.ValidUntil, &f.CreatedAt, &f.UpdatedAt, &f.DeletedAt, &f.Tags,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("scan confirmed fact: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("commit confirm: %w", err)
	}

	return &h, &f, nil
}

// RejectHypothesis rejects a hypothesis with a reason.
func (b *Brain) RejectHypothesis(ctx context.Context, id int64, reason string) (*models.Hypothesis, error) {
	current, err := b.GetHypothesis(ctx, id)
	if err != nil {
		return nil, err
	}

	if !isValidTransition(current.Status, "rejected") {
		return nil, fmt.Errorf("%w: %s → rejected", ErrInvalidHypothesisTransition, current.Status)
	}

	now := time.Now().UTC()
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	var h models.Hypothesis
	err = b.pool.QueryRow(ctx,
		`UPDATE hypotheses SET status = 'rejected', rejected_at = $2, rejection_reason = $3, updated_at = $2
		 WHERE id = $1
		 RETURNING id, namespace_id, content, confidence, status, verification_plan, method,
		 confirmed_fact_id, rejection_reason, source_fact_ids, tested_at, confirmed_at, rejected_at,
		 created_at, updated_at, deleted_at, counter_of`,
		id, now, reasonPtr,
	).Scan(
		&h.ID, &h.NamespaceID, &h.Content, &h.Confidence, &h.Status,
		&h.VerificationPlan, &h.Method, &h.ConfirmedFactID, &h.RejectionReason,
		&h.SourceFactIDs, &h.TestedAt, &h.ConfirmedAt, &h.RejectedAt,
		&h.CreatedAt, &h.UpdatedAt, &h.DeletedAt, &h.CounterOf,
	)
	if err != nil {
		return nil, fmt.Errorf("reject hypothesis: %w", err)
	}
	if err := recordHypothesisEvent(ctx, b.pool, hypothesisEvent{
		HypothesisID: id,
		NamespaceID:  h.NamespaceID,
		Event:        HypothesisEventRejected,
		StatusFrom:   strPtr(current.Status),
		StatusTo:     "rejected",
		Reason:       reasonPtr,
	}); err != nil {
		return nil, err
	}
	return &h, nil
}

// RefineHypothesis updates content/plan/confidence and resets status to proposed.
// Only allowed from testing status.
func (b *Brain) RefineHypothesis(ctx context.Context, id int64, content, verificationPlan string, confidence float32) (*models.Hypothesis, error) {
	current, err := b.GetHypothesis(ctx, id)
	if err != nil {
		return nil, err
	}

	if !isValidTransition(current.Status, "proposed") {
		return nil, fmt.Errorf("%w: can only refine from testing status, current: %s", ErrInvalidHypothesisTransition, current.Status)
	}

	if content == "" {
		content = current.Content
	}
	if verificationPlan == "" {
		verificationPlan = current.VerificationPlan
	}
	if confidence == 0 {
		confidence = current.Confidence
	}

	now := time.Now().UTC
	var h models.Hypothesis
	err = b.pool.QueryRow(ctx,
		`UPDATE hypotheses SET content = $2, verification_plan = $3, confidence = $4,
		 status = 'proposed', tested_at = NULL, updated_at = $5
		 WHERE id = $1
		 RETURNING id, namespace_id, content, confidence, status, verification_plan, method,
		 confirmed_fact_id, rejection_reason, source_fact_ids, tested_at, confirmed_at, rejected_at,
		 created_at, updated_at, deleted_at, counter_of`,
		id, content, verificationPlan, confidence, now,
	).Scan(
		&h.ID, &h.NamespaceID, &h.Content, &h.Confidence, &h.Status,
		&h.VerificationPlan, &h.Method, &h.ConfirmedFactID, &h.RejectionReason,
		&h.SourceFactIDs, &h.TestedAt, &h.ConfirmedAt, &h.RejectedAt,
		&h.CreatedAt, &h.UpdatedAt, &h.DeletedAt, &h.CounterOf,
	)
	if err != nil {
		return nil, fmt.Errorf("refine hypothesis: %w", err)
	}
	if err := recordHypothesisEvent(ctx, b.pool, hypothesisEvent{
		HypothesisID:   id,
		NamespaceID:    h.NamespaceID,
		Event:          HypothesisEventRefined,
		StatusFrom:     strPtr(current.Status),
		StatusTo:       "proposed",
		ConfidenceFrom: &current.Confidence,
		ConfidenceTo:   &confidence,
		Detail:         map[string]any{"content": content, "verification_plan": verificationPlan},
	}); err != nil {
		return nil, err
	}
	return &h, nil
}

// DeleteHypothesis soft-deletes a hypothesis by ID. The row stays (with its
// log) so the history of what was believed is not rewritten by a delete.
func (b *Brain) DeleteHypothesis(ctx context.Context, id int64) error {
	var nsID int64
	var status string
	err := b.pool.QueryRow(ctx,
		"UPDATE hypotheses SET deleted_at = now(), updated_at = now() WHERE id = $1 AND deleted_at IS NULL RETURNING namespace_id, status",
		id,
	).Scan(&nsID, &status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrHypothesisNotFound
		}
		return fmt.Errorf("delete hypothesis: %w", err)
	}
	if err := recordHypothesisEvent(ctx, b.pool, hypothesisEvent{
		HypothesisID: id,
		NamespaceID:  nsID,
		Event:        HypothesisEventDeleted,
		StatusFrom:   strPtr(status),
		StatusTo:     status,
	}); err != nil {
		return err
	}
	return nil
}
