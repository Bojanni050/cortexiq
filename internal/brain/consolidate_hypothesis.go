// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"log"

	"github.com/Bojanni050/cortexiq/internal/models"
)

func (b *Brain) consolidateHypothesisEvidence(ctx context.Context, nsID int64, cp *models.ConsolidationProgress) (autoConfirmed, autoRejected, updated, llmCalls int, errs []string) {
	// Everything below is the sweep's own doing, so the log names it as such
	// instead of falling back to whatever principal the scheduler ran under.
	ctx = WithActor(ctx, ActorConsolidator)

	rows, err := b.pool.Query(ctx,
		`SELECT id, namespace_id, content, confidence, status, verification_plan, method,
		 confirmed_fact_id, rejection_reason, source_fact_ids, tested_at, confirmed_at, rejected_at,
		 created_at, updated_at, deleted_at, counter_of
		 FROM hypotheses WHERE namespace_id = $1 AND status IN ('proposed', 'testing') AND deleted_at IS NULL`,
		nsID,
	)
	if err != nil {
		errs = append(errs, fmt.Sprintf("fetch open hypotheses: %v", err))
		return
	}
	defer rows.Close()

	hypotheses, err := scanHypothesisRows(rows)
	if err != nil {
		errs = append(errs, fmt.Sprintf("scan hypotheses: %v", err))
		return
	}

	if len(hypotheses) == 0 {
		return
	}

	factSQL, factArgs, err := b.queries.FetchFacts(nsID, cp.LastHypothesisFactID, 30)
	if err != nil {
		errs = append(errs, fmt.Sprintf("build fetch facts for hypotheses: %v", err))
		return
	}

	factRows, err := b.pool.Query(ctx, factSQL, factArgs...)
	if err != nil {
		errs = append(errs, fmt.Sprintf("fetch facts for hypotheses: %v", err))
		return
	}
	defer factRows.Close()

	var facts []models.Fact
	for factRows.Next() {
		var f models.Fact
		if err := factRows.Scan(&f.ID, &f.NamespaceID, &f.Content, &f.Embedding, &f.EmbeddingModel, &f.Confidence, &f.Entity, &f.Property, &f.Value, &f.ValidFrom, &f.ValidUntil, &f.CreatedAt, &f.UpdatedAt); err != nil {
			errs = append(errs, fmt.Sprintf("scan fact for hypotheses: %v", err))
			continue
		}
		facts = append(facts, f)
	}
	if err := factRows.Err(); err != nil {
		errs = append(errs, fmt.Sprintf("fact rows for hypotheses: %v", err))
		return
	}

	if len(facts) == 0 {
		return
	}

	llmCalls++
	results, err := b.reasoner.ReasonHypothesisEvidence(ctx, hypotheses, facts)
	if err != nil {
		errs = append(errs, fmt.Sprintf("reason hypothesis evidence: %v", err))
		return
	}

	for _, r := range results {
		hyp, getErr := b.GetHypothesis(ctx, r.HypothesisID)
		if getErr != nil {
			continue
		}

		switch {
		case r.Verdict == "supports" && r.Confidence >= b.config.HypothesisAutoConfirmThreshold:
			switch hyp.Status {
			case "proposed":
				_, statusErr := b.UpdateHypothesisStatus(ctx, r.HypothesisID, "testing")
				if statusErr != nil {
					errs = append(errs, fmt.Sprintf("auto-transition hypothesis %d to testing: %v", r.HypothesisID, statusErr))
					continue
				}
				updated++
			case "testing":
				// A belief the consolidator inferred from facts must never be
				// confirmed by the consolidator: that is memory voting for its own
				// guess with the evidence that produced it. It waits for a human or
				// an agent to confirm it explicitly.
				if hyp.Method == MethodInferred {
					log.Printf("hypothesis %d: not auto-confirming an inferred hypothesis", r.HypothesisID)
					continue
				}
				// A belief and the case against it must never both become facts on
				// the same batch of evidence: that is not a conclusion, it is a
				// contradiction the memory would store twice. When the counterpart
				// is already confirmed, this one waits for a human.
				if counterpartConfirmed, pairErr := b.counterpartIsConfirmed(ctx, hyp); pairErr != nil {
					errs = append(errs, fmt.Sprintf("check counterpart of hypothesis %d: %v", r.HypothesisID, pairErr))
					continue
				} else if counterpartConfirmed {
					log.Printf("hypothesis %d: not auto-confirming, its counterpart is already a fact", r.HypothesisID)
					continue
				}
				_, _, confirmErr := b.ConfirmHypothesis(ctx, r.HypothesisID)
				if confirmErr != nil {
					errs = append(errs, fmt.Sprintf("auto-confirm hypothesis %d: %v", r.HypothesisID, confirmErr))
					continue
				}
				autoConfirmed++
			}

		case r.Verdict == "contradicts" && r.Confidence >= b.config.HypothesisAutoRejectThreshold:
			reason := r.Reasoning
			if reason == "" {
				reason = "Auto-rejected: contradicting evidence detected during consolidation"
			}
			_, rejectErr := b.RejectHypothesis(ctx, r.HypothesisID, reason)
			if rejectErr != nil {
				errs = append(errs, fmt.Sprintf("auto-reject hypothesis %d: %v", r.HypothesisID, rejectErr))
				continue
			}
			autoRejected++

		case r.Verdict == "supports" || r.Verdict == "weakens":
			_, err := b.pool.Exec(ctx,
				`UPDATE hypotheses SET confidence = $2, updated_at = now() WHERE id = $1`,
				r.HypothesisID, r.NewConfidence,
			)
			if err != nil {
				errs = append(errs, fmt.Sprintf("update hypothesis %d confidence: %v", r.HypothesisID, err))
				continue
			}
			// Only a supporting batch adds to the proof count. A weakening batch
			// is evidence too, but counting it as support would make a belief
			// look better the more it was doubted.
			factIDs := factIDsOf(facts)
			if r.Verdict == "supports" && len(factIDs) > 0 {
				if _, err := b.pool.Exec(ctx,
					`UPDATE hypotheses
					    SET supported_by = ARRAY(SELECT DISTINCT unnest(supported_by || $2::bigint[]))
					  WHERE id = $1`,
					r.HypothesisID, factIDs,
				); err != nil {
					errs = append(errs, fmt.Sprintf("update hypothesis %d support: %v", r.HypothesisID, err))
					continue
				}
			}
			// A confidence that drifts without a trace is indistinguishable from
			// one that was edited by hand, so every shift lands in the log.
			reason := r.Reasoning
			if err := recordHypothesisEvent(ctx, b.pool, hypothesisEvent{
				HypothesisID:   r.HypothesisID,
				NamespaceID:    hyp.NamespaceID,
				Event:          HypothesisEventEvidence,
				StatusFrom:     strPtr(hyp.Status),
				StatusTo:       hyp.Status,
				ConfidenceFrom: &hyp.Confidence,
				ConfidenceTo:   &r.NewConfidence,
				Reason:         strPtr(reason),
				Detail:         map[string]any{"verdict": r.Verdict, "llm_confidence": r.Confidence, "facts_considered": len(facts), "fact_ids": factIDs},
			}); err != nil {
				errs = append(errs, fmt.Sprintf("record evidence for hypothesis %d: %v", r.HypothesisID, err))
				continue
			}
			updated++
		}
	}

	var maxFactID int64
	for _, f := range facts {
		if f.ID > maxFactID {
			maxFactID = f.ID
		}
	}
	if maxFactID > cp.LastHypothesisFactID {
		cp.LastHypothesisFactID = maxFactID
	}

	return
}
