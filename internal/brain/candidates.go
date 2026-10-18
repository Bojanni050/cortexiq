// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"sort"
	"strings"
	"unicode"

	"github.com/Bojanni050/cortexiq/internal/models"
)

// clusterCandidate is one episode weighed for membership of a cluster, with the
// score of the arm that proposed it. The arms score on different scales (cosine
// vs. character-trigram Jaccard), so a score means nothing outside its own arm
// and is never compared across arms.
type clusterCandidate struct {
	episode models.Episode
	score   float32
}

// rankCandidates keeps the best limit candidates scoring at least floor, best
// first. A zero score is never kept, whatever the floor: a zero hit is no
// relation at all, not a weak one. limit <= 0 keeps everyone that qualifies.
func rankCandidates(candidates []clusterCandidate, floor float32, limit int) []clusterCandidate {
	kept := make([]clusterCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.score > 0 && c.score >= floor {
			kept = append(kept, c)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].score > kept[j].score })
	if limit > 0 && len(kept) > limit {
		return kept[:limit]
	}
	return kept
}

// lexicalScore is the Jaccard similarity of the character trigrams of two
// texts, on 0..1. It is the second route into a cluster: a reworded or
// translated observation can miss the cosine threshold and still share most of
// its wording. Trigrams rather than words, so inflections and typos still
// count, and the score needs no model, no threshold and no database.
func lexicalScore(a, b string) float32 {
	return trigramJaccard(trigrams(a), trigrams(b))
}

// trigramCache holds the trigram set of each episode of one clustering pass, so
// an episode is tokenised once instead of once per seed. It lives only as long
// as the pass (episode content does not change within it), so nothing needs
// invalidating, and it stays out of the persisted Episode model.
type trigramCache map[int64]map[string]bool

func (c trigramCache) of(e models.Episode) map[string]bool {
	if t, ok := c[e.ID]; ok {
		return t
	}
	t := trigrams(e.Content)
	c[e.ID] = t
	return t
}

// trigramJaccard is the Jaccard similarity of two trigram sets, on 0..1.
func trigramJaccard(ta, tb map[string]bool) float32 {
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	if len(ta) > len(tb) {
		ta, tb = tb, ta
	}
	shared := 0
	for g := range ta {
		if tb[g] {
			shared++
		}
	}
	return float32(shared) / float32(len(ta)+len(tb)-shared)
}

// trigrams returns the set of 3-rune substrings of the normalised text. Every
// run of punctuation or whitespace becomes one space, so "poort 9090" and
// "poort, 9090" are the same text while "poort9090" is a different one. A text
// shorter than three runes has no trigrams.
func trigrams(s string) map[string]bool {
	var b strings.Builder
	b.Grow(len(s))
	space := true
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	runes := []rune(strings.TrimSpace(b.String()))
	out := make(map[string]bool, len(runes))
	for i := 0; i+3 <= len(runes); i++ {
		out[string(runes[i:i+3])] = true
	}
	return out
}
