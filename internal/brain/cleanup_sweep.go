package brain

import (
	"context"
	"fmt"
	"strings"
)

// SweepReport says what one background pass over the duplicate groups did.
type SweepReport struct {
	// Decidable is how many groups the pass found that need no judgement.
	Decidable int `json:"decidable"`
	// Merged is how many of those were actually folded together. A group whose
	// spellings all read the same is not a merge and is not counted.
	Merged int `json:"merged"`
	// Facts and Relations count what the merges rewrote.
	Facts     int `json:"facts"`
	Relations int `json:"relations"`
	// Failed counts the groups a merge refused; the pass went on with the rest.
	Failed int `json:"failed"`
	// FirstError is the reason the first of those failed, for the log line.
	FirstError string `json:"first_error,omitempty"`
}

// SweepCleanup folds the duplicate groups that need no judgement — the ones that
// differ only in capitals or by a leading "the" — into the spelling the system
// writes itself. It is the decision the Clean-up screen offers, taken without
// anyone to press the button.
//
// It stays on the safe side of that decision in two ways. It only touches a group
// whose Keep the server computed for it (canonicalSpelling), so a group that
// differs in punctuation or carries a "(note)" is never touched. And it merges one
// group at a time through MergeEntities, so the same transaction rules apply as
// when a person presses the button: fact texts, embeddings and contradictions are
// left alone.
func (b *Brain) SweepCleanup(ctx context.Context, namespaceSlugs []string) (SweepReport, error) {
	var rep SweepReport
	c, err := b.Cleanup(ctx, namespaceSlugs, CleanupOptions{})
	if err != nil {
		return rep, err
	}
	for _, g := range c.Duplicates {
		if g.Keep == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		rep.Decidable++
		others := mergeTargets(g)
		if len(others) == 0 {
			continue
		}
		m, err := b.MergeEntities(ctx, namespaceSlugs, g.Keep, others, false)
		if err != nil {
			rep.Failed++
			if rep.FirstError == "" {
				rep.FirstError = fmt.Sprintf("%s: %v", g.Keep, err)
			}
			continue
		}
		rep.Merged++
		rep.Facts += m.Facts
		rep.Relations += m.Relations
	}
	return rep, nil
}

// mergeTargets lists the spellings a decidable group folds away: everything that
// does not already read as the kept spelling. A group without a Keep has nothing
// to fold, which is how punctuation and "(note)" groups stay out of the sweep.
//
// The comparison is exact on purpose. Keep is the canonical spelling, so a variant
// that only differs in capitals is exactly what has to be folded in; comparing
// without capitals would empty every case group and merge nothing.
func mergeTargets(g DuplicateGroup) []string {
	if g.Keep == "" {
		return nil
	}
	others := make([]string, 0, len(g.Variants))
	for _, v := range g.Variants {
		if strings.TrimSpace(v.Name) != g.Keep {
			others = append(others, v.Name)
		}
	}
	return others
}
