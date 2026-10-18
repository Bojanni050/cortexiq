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

// candidateArm is one route into a cluster: the episodes that arm proposes,
// best first.
type candidateArm struct {
	name       string
	candidates []clusterCandidate
}

// interleaveCandidates merges the arms round-robin — all #1s first, then all
// #2s — and returns at most limit episodes.
//
// Reciprocal rank fusion sums a hit's reciprocal ranks over the arms, so a hit
// that one arm ranks first but the others do not rank at all is averaged down
// and can fall outside the budget. Interleave gives every arm's top hits a slot
// instead. That matters for clustering because the twin of an episode — the
// observation the extractor has to see next to it to merge it instead of
// writing a duplicate — can be the top hit of one arm on its own.
func interleaveCandidates(arms []candidateArm, limit int) []models.Episode {
	if limit <= 0 {
		return nil
	}
	seen := make(map[int64]bool)
	out := make([]models.Episode, 0, limit)
	for depth := 0; len(out) < limit; depth++ {
		proposed := false
		for _, arm := range arms {
			if depth >= len(arm.candidates) {
				continue
			}
			proposed = true
			c := arm.candidates[depth]
			if seen[c.episode.ID] {
				continue
			}
			seen[c.episode.ID] = true
			out = append(out, c.episode)
			if len(out) == limit {
				break
			}
		}
		if !proposed {
			break
		}
	}
	return out
}

// rankCandidates keeps the best limit candidates of one arm, best first, and
// drops the ones that scored nothing: a zero hit is no relation at all, not a
// weak one. limit <= 0 keeps everyone that scored.
func rankCandidates(candidates []clusterCandidate, limit int) []clusterCandidate {
	kept := make([]clusterCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.score > 0 {
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
	ta, tb := trigrams(a), trigrams(b)
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
