package brain

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestKeywordQuery(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"only stopwords", "the of a", ""},
		{"dutch question", "Welke poort draait cortexiq?", "welke or poort or draait or cortexiq"},
		{"numbers kept", "cortexiq port 8445", "cortexiq or port or 8445"},
		{"deduplicated", "cortexiq  cortexiq CORTEXIQ", "cortexiq"},
		{"punctuation stripped", `Bo's "server" (v1.2)`, "bo or server or v1"},
		{"tsquery syntax neutralised", "x or y; DROP TABLE facts", "drop or table or facts"},
		{"single characters dropped", "a b c", ""},
	}
	for _, tt := range tests {
		if got := keywordQuery(tt.in); got != tt.want {
			t.Errorf("%s: keywordQuery(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestKeywordQueryCapsTerms(t *testing.T) {
	q := "aa bb cc dd ee ff gg hh ii jj kk ll mm nn oo"
	got := keywordQuery(q)
	want := "aa or bb or cc or dd or ee or ff or gg or hh or ii or jj or kk or ll"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func rr(ids ...int64) []RecallResult {
	out := make([]RecallResult, len(ids))
	for i, id := range ids {
		out[i] = RecallResult{ID: id, Score: 0.9 - float32(i)/10}
	}
	return out
}

func ids(rs []RecallResult) []int64 {
	var out []int64
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

func TestFuseNonHybridKeepsVectorOrderAndScore(t *testing.T) {
	got := fuse(rr(3, 1, 2), nil, 2, false, 0)
	if !reflect.DeepEqual(ids(got), []int64{3, 1}) {
		t.Fatalf("ids = %v", ids(got))
	}
	if got[0].Score != 0.9 || got[0].Match != "vector" {
		t.Errorf("score/match = %v/%q", got[0].Score, got[0].Match)
	}
}

func TestFuseHybridRewardsAgreement(t *testing.T) {
	// 5 is top of the vector list only; 7 is 2nd in vector and top in keyword.
	got := fuse(rr(5, 7), rr(7, 9), 10, true, 0)
	if got[0].ID != 7 || got[0].Match != "hybrid" {
		t.Fatalf("expected 7 first as hybrid, got %+v", got[0])
	}
	if got[0].Similarity == 0 {
		t.Error("hybrid vector hit should keep its cosine similarity")
	}
	for _, r := range got {
		if r.Score <= 0 || r.Score > 1 {
			t.Errorf("score %v out of (0,1]", r.Score)
		}
		if r.ID == 9 && (r.Match != "keyword" || r.Similarity != 0) {
			t.Errorf("keyword-only hit: %+v", r)
		}
	}
	if len(got) != 3 {
		t.Errorf("want 3 unique results, got %d", len(got))
	}
}

func TestFuseHybridScaleIndependentOfKeywordHits(t *testing.T) {
	got := fuse(rr(1), nil, 5, true, 0)
	if got[0].Score >= 0.9 || got[0].Match != "vector" {
		t.Errorf("hybrid with no keyword hits should use the fused scale, got %+v", got[0])
	}
}

func TestFuseLimit(t *testing.T) {
	if got := fuse(rr(1, 2, 3), rr(4, 5, 6), 4, true, 0); len(got) != 4 {
		t.Errorf("len = %d, want 4", len(got))
	}
}

func TestRelevantKeepsKeywordConfirmedAndStrongVectorHits(t *testing.T) {
	// rr() gives similarities 0.9, 0.8, 0.7 for ids 1, 2, 3 (vector list) and
	// 0.9, 0.8 for ids 7, 8 (keyword list; their score is a text rank, not cosine).
	got := fuse(rr(1, 2, 3), rr(3, 8), 10, true, 0.75)
	have := map[int64]string{}
	for _, r := range got {
		have[r.ID] = r.Match
	}
	if have[1] != "vector" || have[2] != "vector" {
		t.Errorf("strong vector hits must stay: %v", have)
	}
	if have[3] != "hybrid" {
		t.Errorf("id 3 is below the cutoff but keyword-confirmed, so it must stay as hybrid: %v", have)
	}
	if have[8] != "keyword" {
		t.Errorf("keyword-only hit must stay: %v", have)
	}
}

func TestRelevantDropsWeakVectorOnlyHits(t *testing.T) {
	weak := []RecallResult{{ID: 1, Score: 0.35}, {ID: 2, Score: 0.30}}
	if got := fuse(weak, nil, 10, true, 0.4); len(got) != 0 {
		t.Errorf("weak vector-only hits should be dropped, got %+v", got)
	}
	if got := fuse(weak, nil, 10, false, 0.4); len(got) != 0 {
		t.Errorf("cutoff must also apply without hybrid, got %+v", got)
	}
	if got := fuse(weak, nil, 10, true, 0); len(got) != 2 {
		t.Errorf("cutoff 0 must disable filtering, got %d", len(got))
	}
}

func TestRecencyFactor(t *testing.T) {
	hl := 168 * time.Hour
	if got := recencyFactor(0, 0, hl); got != 1 {
		t.Errorf("weight 0 must not change scores, got %v", got)
	}
	if got := recencyFactor(0, 0.3, hl); math.Abs(got-1) > 1e-9 {
		t.Errorf("brand new result keeps full score, got %v", got)
	}
	// One half-life: half of the 0.3 recency effect is gone.
	if got := recencyFactor(hl, 0.3, hl); math.Abs(got-0.85) > 1e-9 {
		t.Errorf("one half-life: want 0.85, got %v", got)
	}
	// Very old results converge on 1-weight, never below it.
	if got := recencyFactor(100*hl, 0.3, hl); got < 0.7 || got > 0.7001 {
		t.Errorf("very old: want ~0.7, got %v", got)
	}
	if got := recencyFactor(-time.Hour, 0.3, hl); got != 1 {
		t.Errorf("future timestamps count as brand new, got %v", got)
	}
}

func TestApplyBoostsRecencyReordersAndTruncates(t *testing.T) {
	now := time.Now()
	b := &Brain{config: Config{RecencyWeight: 0.5, RecencyHalfLife: 24 * time.Hour}}
	rs := []RecallResult{
		{ID: 1, Score: 0.9, refTime: now.Add(-30 * 24 * time.Hour)},
		{ID: 2, Score: 0.8, refTime: now},
	}
	got := b.applyBoosts(rs, 1, now)
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("recent result should overtake a stale one, got %+v", got)
	}

	off := &Brain{}
	got = off.applyBoosts([]RecallResult{{ID: 1, Score: 0.9}, {ID: 2, Score: 0.95}}, 5, now)
	if got[0].ID != 1 || got[0].Score != 0.9 {
		t.Errorf("disabled recency must leave order and scores alone, got %+v", got)
	}
}

func TestImportanceFactor(t *testing.T) {
	if got := importanceFactor(0.1, 0); got != 1 {
		t.Errorf("weight 0 must not change scores, got %v", got)
	}
	if got := importanceFactor(1, 0.4); math.Abs(got-1) > 1e-9 {
		t.Errorf("importance 1 keeps the full score, got %v", got)
	}
	if got := importanceFactor(0, 0.4); math.Abs(got-0.6) > 1e-9 {
		t.Errorf("importance 0 keeps 1-weight, got %v", got)
	}
	if got := importanceFactor(0.5, 0.4); math.Abs(got-0.8) > 1e-9 {
		t.Errorf("neutral importance: want 0.8, got %v", got)
	}
	if got := importanceFactor(7, 0.4); got != 1 {
		t.Errorf("out-of-range importance is clamped, got %v", got)
	}
}

func TestApplyBoostsImportanceOvertakes(t *testing.T) {
	b := &Brain{config: Config{ImportanceWeight: 0.5}}
	rs := []RecallResult{
		{ID: 1, Score: 0.9, Importance: 0.1},
		{ID: 2, Score: 0.8, Importance: 1},
	}
	got := b.applyBoosts(rs, 5, time.Now())
	if got[0].ID != 2 {
		t.Fatalf("important result should overtake, got %+v", got)
	}
	if got[0].Score > 1 {
		t.Errorf("scores must stay within 0..1, got %v", got[0].Score)
	}
}

// Fact and episode ids are separate sequences that overlap, so every result must
// say which table its id lives in: DELETE /api/v1/facts/<episode id> would take
// an unrelated fact with it.
func TestTypedIDNamesTheTable(t *testing.T) {
	fact := RecallResult{ID: 7}
	fact.typedID("fact")
	if fact.FactID == nil || *fact.FactID != 7 || fact.EpisodeID != nil {
		t.Fatalf("fact result: %+v", fact)
	}

	episode := RecallResult{ID: 7}
	episode.typedID("episode")
	if episode.EpisodeID == nil || *episode.EpisodeID != 7 || episode.FactID != nil {
		t.Fatalf("episode result: %+v", episode)
	}

	blob, err := json.Marshal(episode)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(blob), `"episode_id":7`) || strings.Contains(string(blob), "fact_id") {
		t.Fatalf("episode json = %s", blob)
	}
	blob, err = json.Marshal(fact)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(blob), `"fact_id":7`) || strings.Contains(string(blob), "episode_id") {
		t.Fatalf("fact json = %s", blob)
	}
}

func TestFuseRecordsEachArmsRank(t *testing.T) {
	vec := rr(1, 2, 3) // ranks 1..3 in the vector arm
	kw := rr(3, 9)     // 3 is the keyword arm's top hit; 9 only the keyword arm found
	byID := map[int64]RecallResult{}
	for _, r := range fuse(vec, kw, 10, true, 0) {
		byID[r.ID] = r
	}
	if r := byID[3]; r.vecRank != 3 || r.kwRank != 1 {
		t.Errorf("id 3: vector rank %d, keyword rank %d, want 3 and 1", r.vecRank, r.kwRank)
	}
	if r := byID[9]; r.vecRank != 0 || r.kwRank != 2 {
		t.Errorf("id 9: vector rank %d, keyword rank %d, want 0 (absent) and 2", r.vecRank, r.kwRank)
	}
	if r := byID[1]; r.kwRank != 0 || r.vecRank != 1 {
		t.Errorf("id 1: vector rank %d, keyword rank %d, want 1 and 0 (absent)", r.vecRank, r.kwRank)
	}

	plain := fuse(rr(5, 6), nil, 10, false, 0)
	if plain[0].vecRank != 1 || plain[1].vecRank != 2 {
		t.Errorf("non-hybrid ranks = %d, %d, want 1, 2", plain[0].vecRank, plain[1].vecRank)
	}
}

func TestApplyBoostsRecordsFactorsForTheTrace(t *testing.T) {
	now := time.Now()
	b := &Brain{config: Config{RecencyWeight: 0.5, RecencyHalfLife: 24 * time.Hour, ImportanceWeight: 0.4}}
	got := b.applyBoosts([]RecallResult{{ID: 1, Score: 0.8, Importance: 0, refTime: now}}, 5, now)
	r := got[0]
	if r.fusedScore != 0.8 {
		t.Errorf("fused score = %v, want the score before boosts (0.8)", r.fusedScore)
	}
	if math.Abs(r.importanceF-0.6) > 1e-9 || math.Abs(r.recencyF-1) > 1e-9 {
		t.Errorf("factors = recency %v, importance %v, want 1 and 0.6", r.recencyF, r.importanceF)
	}
	if tr := r.trace(); tr.FusedScore != 0.8 || tr.ImportanceFactor != 0.6 {
		t.Errorf("trace = %+v", tr)
	}

	off := (&Brain{}).applyBoosts([]RecallResult{{ID: 1, Score: 0.7}}, 5, now)
	if off[0].recencyF != 1 || off[0].importanceF != 1 || off[0].fusedScore != 0.7 {
		t.Errorf("with weighting off the factors must be 1, got %+v", off[0])
	}
}

func TestRecallOptionsIncludes(t *testing.T) {
	for _, tt := range []struct {
		types           []string
		facts, episodes bool
	}{
		{nil, true, true},
		{[]string{"fact"}, true, false},
		{[]string{"episode"}, false, true},
		{[]string{"fact", "episode"}, true, true},
	} {
		f, e := RecallOptions{Types: tt.types}.includes()
		if f != tt.facts || e != tt.episodes {
			t.Errorf("types %v: facts %v episodes %v, want %v %v", tt.types, f, e, tt.facts, tt.episodes)
		}
	}
}

func TestNilTraceRecordsNothing(t *testing.T) {
	var tr *RecallTrace
	stage := tr.stage(time.Now())
	stage("embed", 1) // must not panic
}

func TestRecallTraceStagesAreRecordedInOrder(t *testing.T) {
	tr := &RecallTrace{}
	stage := tr.stage(time.Now())
	stage("embed", 0)
	stage("facts_vector", 12)
	if len(tr.Stages) != 2 || tr.Stages[0].Name != "embed" || tr.Stages[1].Count != 12 {
		t.Errorf("stages = %+v", tr.Stages)
	}
}
