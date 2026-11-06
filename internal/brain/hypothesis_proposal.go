// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"log"

	"github.com/Bojanni050/cortexiq/internal/models"
)

// MethodInferred marks a belief the consolidator proposed itself, from facts,
// without an agent or a human asserting it. It is the one method that is never
// auto-confirmed: memory must not turn its own guess into a fact on the strength
// of the evidence that produced the guess.
const MethodInferred = "inferred"

const (
	// maxProposalFacts bounds what one proposal call sees. A longer list makes
	// the model hunt for something to say; a short one keeps it to what the last
	// run actually produced.
	maxProposalFacts = 25
	// maxKnownHypotheses bounds the dedup list. Old rejected beliefs still count
	// — re-proposing them is how a log fills with the same doubt twice.
	maxKnownHypotheses = 40
)

// consolidateHypothesisProposals is the stage that lets memory notice what it
// does not know. The evidence stage only tests beliefs that already exist, so
// without this a namespace whose agents never call create_hypothesis never has
// a single hypothesis to test — the exact silence that looked like a bug.
//
// Everything here is deliberately timid: at most a couple of proposals per run,
// capped confidence, and no path to auto-confirmation. The critic then argues
// the other side of every proposal, so the machine's guess arrives already
// challenged.
func (b *Brain) consolidateHypothesisProposals(ctx context.Context, nsID int64, cp *models.ConsolidationProgress) (proposed, llmCalls int, errs []string) {
	if !b.config.HypothesisAutoPropose {
		return
	}
	max := b.config.HypothesisMaxProposals
	if max <= 0 {
		max = 2
	}

	factSQL, factArgs, err := b.queries.FetchFacts(nsID, cp.LastHypothesisProposalFactID, maxProposalFacts)
	if err != nil {
		errs = append(errs, fmt.Sprintf("build fetch facts for proposals: %v", err))
		return
	}
	rows, err := b.pool.Query(ctx, factSQL, factArgs...)
	if err != nil {
		errs = append(errs, fmt.Sprintf("fetch facts for proposals: %v", err))
		return
	}
	defer rows.Close()

	var facts []models.Fact
	for rows.Next() {
		var f models.Fact
		if err := rows.Scan(&f.ID, &f.NamespaceID, &f.Content, &f.Embedding, &f.EmbeddingModel, &f.Confidence, &f.Entity, &f.Property, &f.Value, &f.ValidFrom, &f.ValidUntil, &f.CreatedAt, &f.UpdatedAt); err != nil {
			errs = append(errs, fmt.Sprintf("scan fact for proposals: %v", err))
			continue
		}
		facts = append(facts, f)
	}
	if err := rows.Err(); err != nil {
		errs = append(errs, fmt.Sprintf("fact rows for proposals: %v", err))
		return
	}
	if len(facts) == 0 {
		return
	}

	known, err := b.knownHypothesisContents(ctx, nsID)
	if err != nil {
		errs = append(errs, fmt.Sprintf("fetch known hypotheses: %v", err))
		return
	}

	llmCalls++
	proposals, err := b.reasoner.ProposeHypotheses(ctx, facts, known, max)
	if err != nil {
		// The cursor stays put on purpose: a failed call should see the same
		// facts next run instead of silently skipping them forever.
		errs = append(errs, fmt.Sprintf("propose hypotheses: %v", err))
		return
	}

	// The facts were considered even when the model found nothing worth testing,
	// so the cursor advances here and not per created hypothesis.
	cp.LastHypothesisProposalFactID = facts[len(facts)-1].ID
	if len(proposals) == 0 {
		return
	}

	ctx = WithActor(ctx, ActorConsolidator)
	for _, p := range proposals {
		_, err := b.insertHypothesis(ctx, hypothesisInput{
			NamespaceID:      nsID,
			Content:          p.Content,
			VerificationPlan: p.VerificationPlan,
			Confidence:       p.Confidence,
			SourceFactIDs:    p.SourceFactIDs,
			Method:           MethodInferred,
			Detail:           map[string]any{"reasoning": p.Reasoning},
		})
		if err != nil {
			errs = append(errs, fmt.Sprintf("insert inferred hypothesis: %v", err))
			continue
		}
		proposed++
	}
	if proposed > 0 {
		log.Printf("hypothesis proposals: namespace %d, %d proposed from %d facts", nsID, proposed, len(facts))
	}
	return
}

// knownHypothesisContents returns what must not be proposed again: every
// hypothesis still on record, whatever its status. A rejected belief stays in
// the list on purpose — the lifecycle already keeps it to prevent exactly this.
func (b *Brain) knownHypothesisContents(ctx context.Context, nsID int64) ([]string, error) {
	rows, err := b.pool.Query(ctx,
		`SELECT content FROM hypotheses
		 WHERE namespace_id = $1 AND deleted_at IS NULL
		 ORDER BY id DESC LIMIT $2`,
		nsID, maxKnownHypotheses,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		out = append(out, content)
	}
	return out, rows.Err()
}
