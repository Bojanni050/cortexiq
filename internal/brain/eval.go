package brain

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Labels of the dedup calibration set (eval/dedup_pairs.jsonl).
const (
	// PairSame is a rewording or translation: the pair must end up as one fact.
	PairSame = "same"
	// PairNearMiss shares a subject with its twin but differs in a value, a scope
	// or a detail: merging it loses information.
	PairNearMiss = "near-miss"
	// PairDifferent is an unrelated claim.
	PairDifferent = "different"
)

// DedupPair is one labelled pair of the dedup calibration set.
type DedupPair struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	A     string `json:"a"`
	B     string `json:"b"`
	Note  string `json:"note,omitempty"`
}

// LoadDedupPairs reads a JSON Lines calibration set. Blank lines are skipped;
// a pair with an unknown label, a missing text or a repeated ID is an error, so
// a typo cannot silently drop out of the scores.
func LoadDedupPairs(r io.Reader) ([]DedupPair, error) {
	var pairs []DedupPair
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var p DedupPair
		if err := json.Unmarshal([]byte(text), &p); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		switch p.Label {
		case PairSame, PairNearMiss, PairDifferent:
		default:
			return nil, fmt.Errorf("line %d (%s): label %q is not same, near-miss or different", line, p.ID, p.Label)
		}
		if p.ID == "" || strings.TrimSpace(p.A) == "" || strings.TrimSpace(p.B) == "" {
			return nil, fmt.Errorf("line %d: id, a and b are required", line)
		}
		if seen[p.ID] {
			return nil, fmt.Errorf("line %d: duplicate id %q", line, p.ID)
		}
		seen[p.ID] = true
		pairs = append(pairs, p)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return nil, fmt.Errorf("no pairs in the calibration set")
	}
	return pairs, nil
}

// PairScore is one scored pair.
type PairScore struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	Similarity float32 `json:"similarity"`
	// Decision is what the vector test alone concludes at the configured
	// thresholds: duplicate, review or new.
	Decision string `json:"decision"`
	// LLMSame is the review verdict, only set for pairs in the review band when
	// the LLM review was run.
	LLMSame *bool `json:"llm_same,omitempty"`
}

// LabelStats is the cosine distribution of one label.
type LabelStats struct {
	Count  int     `json:"count"`
	Min    float32 `json:"min"`
	Median float32 `json:"median"`
	Max    float32 `json:"max"`
}

// SweepRow counts the pairs at or above a threshold, per label. Same pairs above
// it are merged correctly; near-miss and different pairs above it are merged
// wrongly.
type SweepRow struct {
	Threshold float32 `json:"threshold"`
	Same      int     `json:"same"`
	NearMiss  int     `json:"near_miss"`
	Different int     `json:"different"`
}

// CurrentOutcome is what the configured thresholds do with the set.
type CurrentOutcome struct {
	High   float32 `json:"high"`
	Review float32 `json:"review"`
	// AutoMergedSame and AutoMergedOther are pairs at or above High, merged
	// without asking. Other counts near-miss and different pairs: a false merge
	// loses information, so this must be 0.
	AutoMergedSame  int `json:"auto_merged_same"`
	AutoMergedOther int `json:"auto_merged_other"`
	// ReviewedSame and ReviewedOther fall in the review band and are left to the
	// LLM.
	ReviewedSame  int `json:"reviewed_same"`
	ReviewedOther int `json:"reviewed_other"`
	// MissedSame are twins below Review: no test ever merges them.
	MissedSame int `json:"missed_same"`
	// LLM is only set when the review was run.
	LLM *LLMOutcome `json:"llm,omitempty"`
}

// LLMOutcome is how the LLM judged the review band.
type LLMOutcome struct {
	Calls int `json:"calls"`
	// SameAccepted are twins it correctly merged; SameRejected twins it kept apart.
	SameAccepted int `json:"same_accepted"`
	SameRejected int `json:"same_rejected"`
	// OtherAccepted are near-misses or unrelated pairs it wrongly merged.
	OtherAccepted int `json:"other_accepted"`
	OtherRejected int `json:"other_rejected"`
}

// DedupEvalReport is the result of scoring a calibration set.
type DedupEvalReport struct {
	Model   string                `json:"model"`
	Pairs   int                   `json:"pairs"`
	Labels  map[string]LabelStats `json:"labels"`
	Sweep   []SweepRow            `json:"sweep"`
	Current CurrentOutcome        `json:"current"`
	// FalseMerges lists the near-miss and different pairs at or above High.
	FalseMerges []PairScore `json:"false_merges,omitempty"`
	// Missed lists the same pairs below Review.
	Missed []PairScore `json:"missed,omitempty"`
	Scores []PairScore `json:"scores"`
}

// ScoreDedupPairs turns the cosine of every pair into a report. sims[i] belongs
// to pairs[i]. It is pure: embedding and the LLM review happen in EvalDedup.
func ScoreDedupPairs(pairs []DedupPair, sims []float32, high, review float32) DedupEvalReport {
	rep := DedupEvalReport{
		Pairs:   len(pairs),
		Labels:  map[string]LabelStats{},
		Current: CurrentOutcome{High: high, Review: review},
	}
	byLabel := map[string][]float32{}
	for i, p := range pairs {
		s := PairScore{ID: p.ID, Label: p.Label, Similarity: sims[i]}
		switch decideDedup(sims[i], high, review) {
		case dedupDuplicate:
			s.Decision = "duplicate"
			if p.Label == PairSame {
				rep.Current.AutoMergedSame++
			} else {
				rep.Current.AutoMergedOther++
				rep.FalseMerges = append(rep.FalseMerges, s)
			}
		case dedupReview:
			s.Decision = "review"
			if p.Label == PairSame {
				rep.Current.ReviewedSame++
			} else {
				rep.Current.ReviewedOther++
			}
		default:
			s.Decision = "new"
			if p.Label == PairSame {
				rep.Current.MissedSame++
				rep.Missed = append(rep.Missed, s)
			}
		}
		byLabel[p.Label] = append(byLabel[p.Label], sims[i])
		rep.Scores = append(rep.Scores, s)
	}

	for label, vals := range byLabel {
		sorted := append([]float32(nil), vals...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		rep.Labels[label] = LabelStats{
			Count: len(sorted), Min: sorted[0], Median: median(sorted), Max: sorted[len(sorted)-1],
		}
	}

	for t := 50; t <= 95; t += 5 {
		th := float32(t) / 100
		row := SweepRow{Threshold: th}
		for i, p := range pairs {
			if sims[i] < th {
				continue
			}
			switch p.Label {
			case PairSame:
				row.Same++
			case PairNearMiss:
				row.NearMiss++
			default:
				row.Different++
			}
		}
		rep.Sweep = append(rep.Sweep, row)
	}
	return rep
}

// median of an ascending slice.
func median(sorted []float32) float32 {
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// DedupEvalOptions controls EvalDedup.
type DedupEvalOptions struct {
	// High and Review are the thresholds to score against; zero takes the ones
	// the brain is configured with.
	High, Review float32
	// LLMReview also asks the reasoner about the pairs in the review band, to
	// measure how well it separates twins from near-misses. It costs one call per
	// pair in the band.
	LLMReview bool
}

// EvalDedup embeds both sides of every pair with the configured embedder and
// scores the set. Nothing is written to the database.
func (b *Brain) EvalDedup(ctx context.Context, pairs []DedupPair, opts DedupEvalOptions) (DedupEvalReport, error) {
	high, review := opts.High, opts.Review
	if high == 0 {
		high = float32(b.config.DedupThreshold)
	}
	if review == 0 {
		review = float32(b.config.DedupReviewThreshold)
	}

	sims := make([]float32, len(pairs))
	for i, p := range pairs {
		va, err := b.embedder.Embed(ctx, p.A)
		if err != nil {
			return DedupEvalReport{}, fmt.Errorf("embed %s a: %w", p.ID, err)
		}
		vb, err := b.embedder.Embed(ctx, p.B)
		if err != nil {
			return DedupEvalReport{}, fmt.Errorf("embed %s b: %w", p.ID, err)
		}
		sims[i] = cosineSimilarity(va, vb)
	}

	rep := ScoreDedupPairs(pairs, sims, high, review)
	rep.Model = b.embedder.Model()

	if opts.LLMReview {
		llm := &LLMOutcome{}
		for i, s := range rep.Scores {
			if s.Decision != "review" {
				continue
			}
			same, err := b.reasoner.ReasonSameFact(ctx, pairs[i].A, pairs[i].B)
			if err != nil {
				return rep, fmt.Errorf("review %s: %w", s.ID, err)
			}
			llm.Calls++
			rep.Scores[i].LLMSame = &same
			switch {
			case s.Label == PairSame && same:
				llm.SameAccepted++
			case s.Label == PairSame:
				llm.SameRejected++
			case same:
				llm.OtherAccepted++
			default:
				llm.OtherRejected++
			}
		}
		rep.Current.LLM = llm
	}
	return rep, nil
}
