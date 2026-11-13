// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import "strings"

// Cues that say a question is about failures or goals rather than only about
// facts. They are Dutch and English, whole words or short phrases, and they are
// deliberately narrow: a missed cue only means the caller has to name the type
// itself, while a false cue adds keyword hits from a second table.
var (
	failureCues = []string{
		"failed", "failure", "failures", "fail", "tried", "attempted", "didnt work", "did not work", "doesnt work",
		"mislukt", "mislukte", "mislukking", "mislukkingen", "geprobeerd", "probeerden", "niet gewerkt", "werkte niet",
		"lesson", "lessons", "geleerd",
	}
	goalCues = []string{
		"goal", "goals", "objective", "objectives", "roadmap", "milestone", "milestones", "todo",
		"doel", "doelen", "doelstelling", "doelstellingen",
	}
)

// routeTypes returns the extra stores a query's wording points to: "failure"
// and/or "goal". It never returns "fact" or "episode" and never removes a store,
// so routing can only widen what a recall looks at. Matching is on whole words
// of the lower-cased query with punctuation (apostrophes included) dropped.
func routeTypes(query string) []string {
	norm := cueText(query)

	var out []string
	if hasCue(norm, failureCues) {
		out = append(out, "failure")
	}
	if hasCue(norm, goalCues) {
		out = append(out, "goal")
	}
	return out
}

// cueText lower-cases a query, drops apostrophes and punctuation and pads it with
// spaces, so a cue can be matched as a whole word or phrase.
func cueText(query string) string {
	return " " + strings.Join(strings.FieldsFunc(strings.ToLower(strings.NewReplacer("'", "", "’", "").Replace(query)), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r < 0x80
	}), " ") + " "
}

// relationCues say a question is about how things connect rather than about one
// topic. Like the cues above they are narrow, whole words or short phrases, and
// Dutch and English: a missed cue only leaves the graph arm at its topical weight.
var relationCues = []string{
	"relation", "relations", "relationship", "relationships", "related", "connected", "connection", "between",
	"depends on", "dependency", "dependencies", "belongs to", "part of", "used by", "linked to",
	"relatie", "relaties", "verband", "verbonden", "gerelateerd", "tussen", "hangt af van", "afhankelijk van",
	"afhankelijkheid", "afhankelijkheden", "hoort bij", "onderdeel van", "gebruikt door", "gekoppeld aan",
}

// isRelational reports whether a question looks relational: it names two or more
// known entities, or its wording asks how things connect. It is a deterministic
// profile of the query, with no model involved. It only steers how much the graph
// arm counts in the fusion; it never removes an arm.
func isRelational(query string, entityCount int) bool {
	return entityCount >= 2 || hasCue(cueText(query), relationCues)
}

const (
	// defaultGraphWeightTopic is the graph arm's weight for a topical question: a
	// fact that shares an entity with the question already has a vector rank, and
	// equal-weight fusion counts it twice (see docs/STATUS.md, graph arm).
	defaultGraphWeightTopic = 0.1
	// defaultGraphWeightRelational is the weight for a relational question.
	defaultGraphWeightRelational = 0.5
)

// graphArmWeight is the weight the graph arm counts with in the fusion for a
// question of the given shape. A weight that is not set (zero or less) takes its
// default.
func (c Config) graphArmWeight(relational bool) float64 {
	w, def := c.GraphWeightTopic, defaultGraphWeightTopic
	if relational {
		w, def = c.GraphWeightRelational, defaultGraphWeightRelational
	}
	if w <= 0 {
		return def
	}
	return w
}

func hasCue(norm string, cues []string) bool {
	for _, c := range cues {
		if strings.Contains(norm, " "+c+" ") {
			return true
		}
	}
	return false
}
