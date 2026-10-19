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
	norm := " " + strings.Join(strings.FieldsFunc(strings.ToLower(strings.NewReplacer("'", "", "’", "").Replace(query)), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r < 0x80
	}), " ") + " "

	var out []string
	if hasCue(norm, failureCues) {
		out = append(out, "failure")
	}
	if hasCue(norm, goalCues) {
		out = append(out, "goal")
	}
	return out
}

func hasCue(norm string, cues []string) bool {
	for _, c := range cues {
		if strings.Contains(norm, " "+c+" ") {
			return true
		}
	}
	return false
}
