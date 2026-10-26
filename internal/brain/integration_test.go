package brain

// Integration tests against a real Postgres (with pgvector). They are skipped
// unless CORTEXIQ_TEST_DSN is set, e.g.
//
//	CORTEXIQ_TEST_DSN=postgres://cortexiq:cortexiq@localhost:5432/cortexiq_test?sslmode=disable go test ./internal/brain/
//
// Use a throw-away database: migrations are applied and every test creates its
// own uniquely named namespace, but nothing is cleaned up afterwards. CI runs
// these against a pgvector service container.

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/Bojanni050/cortexiq/internal/db"
	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/Bojanni050/cortexiq/internal/queries"
	"github.com/Bojanni050/cortexiq/internal/reasoner"
	"github.com/pgvector/pgvector-go"
)

const testDim = 32

// fakeEmbedder is a deterministic bag-of-words hashing embedder: texts that
// share words get similar vectors, identical texts get identical vectors.
type fakeEmbedder struct{}

func (fakeEmbedder) Model() string { return "fake" }
func (fakeEmbedder) Dims() int     { return testDim }
func (fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	v := make([]float32, testDim)
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, w := range words {
		h := fnv.New32a()
		_, _ = h.Write([]byte(w))
		v[h.Sum32()%testDim]++
	}
	var norm float64
	for _, x := range v {
		norm += float64(x * x)
	}
	if norm == 0 {
		v[0], norm = 1, 1
	}
	for i := range v {
		v[i] /= float32(math.Sqrt(norm))
	}
	return v, nil
}

// fakeReasoner implements just enough for the pipeline and records what it saw.
type fakeReasoner struct {
	mu          sync.Mutex
	causalCalls [][]int64 // fact IDs passed to each ReasonCausalLinks call
	subjects    []string  // subject passed to each ReasonStructured call
	sameAnswer  bool      // what ReasonSameFact answers
	sameCalls   int
	rels        map[int64][]*reasoner.StructuredRelationship // what ReasonRelationshipsBatch returns, by fact ID
	patterns    []*reasoner.StructuredPattern                // what ReasonPatterns returns
	chatAnswer  string                                       // what Chat returns
	chatCalls   int
}

// Chat returns the canned chatAnswer, or a default grounded answer.
func (r *fakeReasoner) Chat(_ context.Context, messages []reasoner.ChatMessage) (string, error) {
	r.mu.Lock()
	r.chatCalls++
	r.mu.Unlock()
	if r.chatAnswer != "" {
		return r.chatAnswer, nil
	}
	return "Based on the memories, here is the answer.", nil
}

// ReasonStructured fakes the synthesis: the text is split on " || " into one fact
// per part, and the text "NOFACT" yields no facts at all.
func (r *fakeReasoner) ReasonStructured(_ context.Context, texts []string, subject string) ([]*reasoner.StructuredFact, error) {
	r.mu.Lock()
	r.subjects = append(r.subjects, subject)
	r.mu.Unlock()
	if texts[0] == "NOFACT" {
		return nil, nil
	}
	var facts []*reasoner.StructuredFact
	for _, part := range strings.Split(texts[0], " || ") {
		facts = append(facts, &reasoner.StructuredFact{Summary: part})
	}
	return facts, nil
}
func (r *fakeReasoner) ReasonSameFact(context.Context, string, string) (bool, error) {
	r.mu.Lock()
	r.sameCalls++
	r.mu.Unlock()
	return r.sameAnswer, nil
}
func (r *fakeReasoner) ReasonRelationshipsBatch(context.Context, []models.Fact) (map[int64][]*reasoner.StructuredRelationship, error) {
	return r.rels, nil
}
func (r *fakeReasoner) ReasonPatterns(context.Context, []models.Fact, []models.Relationship) ([]*reasoner.StructuredPattern, error) {
	return r.patterns, nil
}
func (r *fakeReasoner) ReasonContradiction(context.Context, string, string, string, string) (*reasoner.ContradictionResult, error) {
	return &reasoner.ContradictionResult{Classification: reasoner.ClassificationCompatible}, nil
}
func (r *fakeReasoner) ReasonCausalLinks(_ context.Context, facts []models.Fact) ([]*reasoner.StructuredCausalLink, error) {
	ids := make([]int64, len(facts))
	for i, f := range facts {
		ids[i] = f.ID
	}
	r.mu.Lock()
	r.causalCalls = append(r.causalCalls, ids)
	r.mu.Unlock()
	return nil, nil
}
func (r *fakeReasoner) ReasonGoalProgress(context.Context, []models.Goal, []models.Fact) ([]*reasoner.GoalProgressAssessment, error) {
	return nil, nil
}
func (r *fakeReasoner) ReasonFailurePatterns(context.Context, []models.Failure, []string) ([]*reasoner.FailurePatternResult, error) {
	return nil, nil
}
func (r *fakeReasoner) ReasonHypothesisEvidence(context.Context, []models.Hypothesis, []models.Fact) ([]*reasoner.HypothesisEvidenceResult, error) {
	return nil, nil
}

type testEnv struct {
	b    *Brain
	r    *fakeReasoner
	ns   string
	nsID int64
	ctx  context.Context
}

func newEnv(t *testing.T, mutate func(*Config)) *testEnv {
	t.Helper()
	dsn := os.Getenv("CORTEXIQ_TEST_DSN")
	if dsn == "" {
		t.Skip("CORTEXIQ_TEST_DSN not set; skipping integration test")
	}
	ctx := context.Background()

	pool, err := db.Open(ctx, dsn, "fake", testDim)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(pool.Close)

	q, err := queries.New()
	if err != nil {
		t.Fatalf("queries.New: %v", err)
	}
	cfg := DefaultConfig()
	if mutate != nil {
		mutate(&cfg)
	}
	r := &fakeReasoner{}
	b, err := New(pool, fakeEmbedder{}, r, q, cfg)
	if err != nil {
		t.Fatalf("brain.New: %v", err)
	}

	// Segment must match ^[a-z0-9][a-z0-9_-]{0,63}$ and stay unique per test run.
	name := strings.ToLower(t.Name())
	if len(name) > 24 {
		name = name[:24]
	}
	ns := fmt.Sprintf("/it-%s-%d", name, time.Now().UnixNano())
	nsID, err := b.CreateNamespace(ctx, ns, ns, "")
	if err != nil {
		t.Fatalf("CreateNamespace(%q): %v", ns, err)
	}
	return &testEnv{b: b, r: r, ns: ns, nsID: nsID, ctx: ctx}
}

// insertFact inserts a fact directly (bypassing consolidation) and returns its ID.
func (e *testEnv) insertFact(t *testing.T, content string, validUntil *time.Time) int64 {
	t.Helper()
	vec, _ := fakeEmbedder{}.Embed(e.ctx, content)
	var id int64
	err := e.b.pool.QueryRow(e.ctx,
		`INSERT INTO facts (namespace_id, content, embedding, embedding_model, confidence, valid_from, valid_until)
		 VALUES ($1, $2, $3, 'fake', 0.8, now(), $4) RETURNING id`,
		e.nsID, content, pgvector.NewVector(vec), validUntil,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert fact: %v", err)
	}
	return id
}

// Regression: the causal stage used to share LastFactID with the relationships
// stage, which had already advanced it, so causal saw (almost) no facts.
func TestIntegrationCausalStageSeesNewFactsOnce(t *testing.T) {
	e := newEnv(t, nil)
	a := e.insertFact(t, "alpha service restarted", nil)
	b2 := e.insertFact(t, "beta queue backed up", nil)
	c := e.insertFact(t, "gamma alert fired", nil)

	if _, err := e.b.ConsolidateByID(e.ctx, e.nsID); err != nil {
		t.Fatal(err)
	}
	if len(e.r.causalCalls) != 1 || len(e.r.causalCalls[0]) != 3 {
		t.Fatalf("causal stage calls = %v, want one call with the 3 new facts (%d,%d,%d)", e.r.causalCalls, a, b2, c)
	}

	// Checkpoint advanced: nothing new, so no further call.
	if _, err := e.b.ConsolidateByID(e.ctx, e.nsID); err != nil {
		t.Fatal(err)
	}
	if len(e.r.causalCalls) != 1 {
		t.Errorf("causal stage re-ran over already processed facts: %v", e.r.causalCalls)
	}
}

func TestIntegrationRecallSkipsExpiredFacts(t *testing.T) {
	e := newEnv(t, nil)
	past := time.Now().Add(-time.Hour)
	live := e.insertFact(t, "cortexiq dashboard runs on port 8445", nil)
	expired := e.insertFact(t, "cortexiq dashboard runs on port 9999", &past)

	res, err := e.b.Recall(e.ctx, []string{e.ns}, "cortexiq dashboard port", 10)
	if err != nil {
		t.Fatal(err)
	}
	var sawLive bool
	for _, r := range res {
		if r.Type == "fact" && r.ID == expired {
			t.Errorf("expired fact %d was recalled", expired)
		}
		if r.Type == "fact" && r.ID == live {
			sawLive = true
		}
	}
	if !sawLive {
		t.Errorf("live fact %d not recalled: %+v", live, res)
	}
}

func TestIntegrationHybridRecallFlagsKeywordMatches(t *testing.T) {
	e := newEnv(t, nil)
	e.insertFact(t, "the tailscale port for the dashboard is 8445", nil)

	res, err := e.b.Recall(e.ctx, []string{e.ns}, "which port is 8445", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 || res[0].Match != "hybrid" {
		t.Fatalf("expected a hybrid match first, got %+v", res)
	}
	if res[0].Score <= 0 || res[0].Score > 1 || res[0].Similarity <= 0 {
		t.Errorf("score/similarity out of range: %+v", res[0])
	}

	// With hybrid off the behaviour is the original pure vector search.
	off := newEnv(t, func(c *Config) { c.HybridRecall = false })
	off.insertFact(t, "the tailscale port for the dashboard is 8445", nil)
	res, err = off.b.Recall(off.ctx, []string{off.ns}, "which port is 8445", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 || res[0].Match != "vector" {
		t.Fatalf("hybrid disabled: expected vector match, got %+v", res)
	}
}

// Regression: the default parser kept hostnames/URLs/paths as one token, so a
// search for "8445" or "dashboard" missed "https://host.ts.net:8445/ui" and
// "GUI/dashboard". Both must be found by the keyword side.
func TestIntegrationKeywordSearchSplitsUrlsAndPaths(t *testing.T) {
	e := newEnv(t, nil)
	e.insertFact(t, "The web GUI/dashboard is at https://server.example.ts.net:8445/ui", nil)

	for _, q := range []string{"8445", "dashboard", "ui"} {
		res, err := e.b.Recall(e.ctx, []string{e.ns}, q, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) == 0 || res[0].Match != "hybrid" {
			t.Errorf("query %q: keyword side did not match the URL/path fact: %+v", q, res)
		}
	}
}

func TestIntegrationRecallDropsIrrelevantResults(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.MinRecallSimilarity = 0.5 })
	relevant := e.insertFact(t, "kubernetes cluster upgrade planned", nil)

	res, err := e.b.Recall(e.ctx, []string{e.ns}, "kubernetes cluster upgrade", 5)
	if err != nil || len(res) == 0 || res[0].ID != relevant {
		t.Fatalf("relevant query lost its result: %+v err=%v", res, err)
	}

	res, err = e.b.Recall(e.ctx, []string{e.ns}, "banana pancake recipe", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Errorf("irrelevant query should return nothing, got %+v", res)
	}
}

func TestIntegrationRecallIsCountedAndDelaysDecay(t *testing.T) {
	e := newEnv(t, nil)
	used := e.insertFact(t, "kubernetes cluster upgrade planned", nil)
	unused := e.insertFact(t, "completely unrelated trivia about bananas", nil)

	// Both facts look stale to decay.
	if _, err := e.b.pool.Exec(e.ctx,
		"UPDATE facts SET updated_at = now() - interval '30 days' WHERE id = ANY($1)", []int64{used, unused}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.Recall(e.ctx, []string{e.ns}, "kubernetes cluster upgrade", 1); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := e.b.pool.QueryRow(e.ctx, "SELECT recall_count FROM facts WHERE id = $1", used).Scan(&count); err != nil || count != 1 {
		t.Fatalf("recall_count = %d (err %v), want 1", count, err)
	}

	if _, err := e.b.DecayConfidence(e.ctx, e.nsID); err != nil {
		t.Fatal(err)
	}
	conf := func(id int64) float32 {
		var c float32
		if err := e.b.pool.QueryRow(e.ctx, "SELECT confidence FROM facts WHERE id = $1", id).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	if got := conf(used); got != 0.8 {
		t.Errorf("recalled fact decayed: confidence %v, want 0.8", got)
	}
	if got := conf(unused); got >= 0.8 {
		t.Errorf("unused stale fact did not decay: confidence %v", got)
	}
}

func TestIntegrationDedupReinforcesExistingFact(t *testing.T) {
	e := newEnv(t, nil)
	text := "the cortexiq server listens on port 8080"

	if _, err := e.b.Remember(e.ctx, e.ns, text, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.ConsolidateByID(e.ctx, e.nsID); err != nil {
		t.Fatal(err)
	}
	var factID int64
	if err := e.b.pool.QueryRow(e.ctx, "SELECT id FROM facts WHERE namespace_id = $1", e.nsID).Scan(&factID); err != nil {
		t.Fatalf("expected one fact after the first consolidation: %v", err)
	}
	if _, err := e.b.pool.Exec(e.ctx,
		"UPDATE facts SET updated_at = now() - interval '30 days', confidence = 0.5 WHERE id = $1", factID); err != nil {
		t.Fatal(err)
	}

	// Same knowledge observed again.
	if _, err := e.b.Remember(e.ctx, e.ns, text, nil); err != nil {
		t.Fatal(err)
	}
	res, err := e.b.ConsolidateByID(e.ctx, e.nsID)
	if err != nil {
		t.Fatal(err)
	}
	if res.FactsDeduplicated != 1 || res.FactsCreated != 0 {
		t.Fatalf("dedup result = %+v, want 1 deduplicated and 0 created", res)
	}

	var conf float32
	var fresh bool
	var sources int
	err = e.b.pool.QueryRow(e.ctx,
		`SELECT confidence, updated_at > now() - interval '1 day',
		        (SELECT count(*) FROM fact_sources WHERE fact_id = $1)
		 FROM facts WHERE id = $1`, factID).Scan(&conf, &fresh, &sources)
	if err != nil {
		t.Fatal(err)
	}
	if conf <= 0.5 || !fresh || sources != 2 {
		t.Errorf("fact not reinforced: confidence=%v fresh=%v sources=%d", conf, fresh, sources)
	}
}

func TestIntegrationExportImportRoundTrip(t *testing.T) {
	e := newEnv(t, nil)
	epID, err := e.b.Remember(e.ctx, e.ns, "deploy happened on tuesday", nil)
	if err != nil {
		t.Fatal(err)
	}
	factID := e.insertFact(t, "production runs on the strato vps", nil)

	var buf bytes.Buffer
	exp, err := e.b.Export(e.ctx, &buf, []string{e.ns}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exp.Episodes != 1 || exp.Facts != 1 {
		t.Fatalf("export = %+v, want 1 episode and 1 fact", exp)
	}
	data := buf.String()

	if _, err := e.b.PurgeEpisode(e.ctx, epID, PurgeOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.PurgeFact(e.ctx, factID, PurgeOptions{}); err != nil {
		t.Fatal(err)
	}

	dry, err := e.b.Import(e.ctx, strings.NewReader(data), ImportOptions{DryRun: true})
	if err != nil || dry.Episodes != 1 || dry.Facts != 1 {
		t.Fatalf("dry run = %+v err=%v", dry, err)
	}
	imp, err := e.b.Import(e.ctx, strings.NewReader(data), ImportOptions{})
	if err != nil || imp.Episodes != 1 || imp.Facts != 1 || imp.Failed != 0 {
		t.Fatalf("import = %+v err=%v", imp, err)
	}
	again, err := e.b.Import(e.ctx, strings.NewReader(data), ImportOptions{})
	if err != nil || again.Skipped != 2 || again.Episodes != 0 || again.Facts != 0 {
		t.Fatalf("second import should be a no-op, got %+v err=%v", again, err)
	}

	// Imported records are searchable: embeddings were recomputed.
	res, err := e.b.Recall(e.ctx, []string{e.ns}, "strato vps production", 3)
	if err != nil || len(res) == 0 {
		t.Fatalf("imported fact not recallable: %+v err=%v", res, err)
	}
}

// insertEntityFact inserts a fact about an entity (bypassing consolidation).
func (e *testEnv) insertEntityFact(t *testing.T, entity, content string, confidence float32) int64 {
	t.Helper()
	vec, _ := fakeEmbedder{}.Embed(e.ctx, content)
	var id int64
	err := e.b.pool.QueryRow(e.ctx,
		`INSERT INTO facts (namespace_id, content, embedding, embedding_model, confidence, entity, valid_from)
		 VALUES ($1, $2, $3, 'fake', $4, $5, now()) RETURNING id`,
		e.nsID, content, pgvector.NewVector(vec), confidence, entity,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert entity fact: %v", err)
	}
	return id
}

func (e *testEnv) insertRelationship(t *testing.T, from, rel, to string, confidence float32) {
	t.Helper()
	_, err := e.b.pool.Exec(e.ctx,
		`INSERT INTO relationships (namespace_id, from_entity, relation_type, to_entity, confidence)
		 VALUES ($1, $2, $3, $4, $5)`,
		e.nsID, from, rel, to, confidence)
	if err != nil {
		t.Fatalf("insert relationship: %v", err)
	}
}

// The graph arm finds a fact that shares no wording and no vocabulary with the
// query, through an entity the query names and one relationship hop.
func TestIntegrationGraphRecallFollowsEntitiesAndNeighbours(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.GraphRecall = true })
	direct := e.insertEntityFact(t, "pgvector", "De extensie levert HNSW-indexen voor nabijheidszoeken.", 0.8)
	neighbour := e.insertEntityFact(t, "PostgreSQL", "Versie zestien draait op de productieserver.", 0.8)
	e.insertEntityFact(t, "Grafana", "Panelen staan op de monitoringpagina.", 0.8)
	e.insertRelationship(t, "pgvector", "extends", "PostgreSQL", 0.9)

	res, trace, err := e.b.RecallDetailed(e.ctx, []string{e.ns}, "wat doet pgvector eigenlijk", 10, RecallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !trace.Graph || len(trace.GraphEntities) != 1 || trace.GraphEntities[0] != "pgvector" {
		t.Fatalf("trace graph = %v, entities = %v, want graph on and [pgvector]", trace.Graph, trace.GraphEntities)
	}
	byID := map[int64]RecallResult{}
	for _, r := range res {
		if r.FactID != nil {
			byID[*r.FactID] = r
		}
	}
	d, ok := byID[direct]
	if !ok || d.Trace.GraphRank != 1 || d.Trace.GraphScore != 1 {
		t.Fatalf("fact about the named entity = %+v, want graph rank 1, score 1", d.Trace)
	}
	n, ok := byID[neighbour]
	if !ok || n.Trace.GraphRank != 2 || n.Trace.GraphScore <= 0 || n.Trace.GraphScore > 0.5 {
		t.Fatalf("fact about the neighbour = %+v, want graph rank 2, score in (0, 0.5]", n)
	}
	for _, r := range res {
		if r.Content == "Panelen staan op de monitoringpagina." && r.Trace.GraphRank != 0 {
			t.Errorf("an unrelated entity must not be reached by the graph arm: %+v", r.Trace)
		}
	}
}

// A neighbour earns its place by sharing an entity, not by answering the
// question. With a similarity floor configured, a neighbour whose content has
// nothing to do with the query is no longer dragged in — which is the noise that
// made the graph arm cost quality on topic questions.
func TestIntegrationGraphRecallDropsDistantNeighbours(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.GraphRecall = true; c.MinRecallSimilarity = 0.999 })
	direct := e.insertEntityFact(t, "pgvector", "De extensie levert HNSW-indexen voor nabijheidszoeken.", 0.8)
	neighbour := e.insertEntityFact(t, "PostgreSQL", "Versie zestien draait op de productieserver.", 0.8)
	e.insertRelationship(t, "pgvector", "extends", "PostgreSQL", 0.9)

	res, _, err := e.b.RecallDetailed(e.ctx, []string{e.ns}, "wat doet pgvector eigenlijk", 10, RecallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var keptDirect bool
	for _, r := range res {
		if r.FactID == nil {
			continue
		}
		switch *r.FactID {
		case direct:
			keptDirect = true
		case neighbour:
			t.Fatalf("a neighbour with no similarity to the query was kept: %+v", r.Trace)
		}
	}
	if !keptDirect {
		t.Fatalf("the fact about the named entity was dropped by the floor: %+v", res)
	}
}

// With the flag off, the graph arm does not run and the ranking is the old one.
func TestIntegrationGraphRecallOffIsTheOldBehaviour(t *testing.T) {
	e := newEnv(t, nil)
	e.insertEntityFact(t, "pgvector", "De extensie levert HNSW-indexen voor nabijheidszoeken.", 0.8)
	_, trace, err := e.b.RecallDetailed(e.ctx, []string{e.ns}, "wat doet pgvector eigenlijk", 10, RecallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if trace.Graph || len(trace.GraphEntities) != 0 {
		t.Errorf("graph arm ran with the flag off: %+v", trace)
	}
}

// Entity names that are regular-expression syntax must match literally, and a
// name inside another word must not match at all.
func TestIntegrationGraphEntityMatchingIsLiteralAndWordBound(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.GraphRecall = true })
	e.insertEntityFact(t, "C++ (stl)", "De standaardbibliotheek heeft containers.", 0.8)
	e.insertEntityFact(t, "app", "Een korte entiteitsnaam.", 0.8)
	e.insertEntityFact(t, "Bojanni050/cortexiq", "De repository staat op GitHub.", 0.8)

	_, trace, err := e.b.RecallDetailed(e.ctx, []string{e.ns}, "vertel over c++ (stl) en de application server en Bojanni050/cortexiq", 10, RecallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, n := range trace.GraphEntities {
		got[n] = true
	}
	if !got["c++ (stl)"] || !got["bojanni050/cortexiq"] {
		t.Errorf("literal entity names not matched: %v", trace.GraphEntities)
	}
	if got["app"] {
		t.Errorf("'app' matched inside 'application': %v", trace.GraphEntities)
	}
}

// Another namespace's graph must not leak into this namespace's recall.
func TestIntegrationGraphRecallStaysInItsNamespace(t *testing.T) {
	a := newEnv(t, func(c *Config) { c.GraphRecall = true })
	a.insertEntityFact(t, "pgvector", "Feit in namespace A.", 0.8)
	b := newEnv(t, func(c *Config) { c.GraphRecall = true })

	res, trace, err := b.b.RecallDetailed(b.ctx, []string{b.ns}, "wat doet pgvector", 10, RecallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.GraphEntities) != 0 {
		t.Errorf("entities from another namespace were matched: %v", trace.GraphEntities)
	}
	for _, r := range res {
		if r.Content == "Feit in namespace A." {
			t.Errorf("a fact from another namespace was recalled: %+v", r)
		}
	}
}

// EvalGraph runs end to end, compares both modes, and leaves nothing behind.
func TestIntegrationEvalGraphRunsAndCleansUp(t *testing.T) {
	e := newEnv(t, nil)
	cf, _ := os.Open("../../eval/graph_corpus.jsonl")
	defer cf.Close()
	facts, rels, err := LoadGraphCorpus(cf)
	if err != nil {
		t.Fatal(err)
	}
	qf, _ := os.Open("../../eval/graph_queries.jsonl")
	defer qf.Close()
	qs, err := LoadGraphQueries(qf, facts)
	if err != nil {
		t.Fatal(err)
	}

	rep, err := e.b.EvalGraph(e.ctx, facts, rels, qs, RecallEvalOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Facts != len(facts) || rep.Without.Overall.Questions != len(qs) || rep.With.Overall.Questions != len(qs) {
		t.Errorf("report = %d facts, %d/%d questions", rep.Facts, rep.Without.Overall.Questions, rep.With.Overall.Questions)
	}
	// The temporary namespace is gone, and with it its facts and relationships.
	var n int
	if err := e.b.pool.QueryRow(e.ctx, `SELECT count(*) FROM namespaces WHERE slug LIKE '/eval/graph-%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d /eval/graph-* namespaces left behind", n)
	}
}

// Within an entity with many equally trusted facts, the graph arm puts the fact
// closest to the question first, whatever the order they were stored in.
func TestIntegrationGraphArmOrdersAnEntitysFactsBySimilarity(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.GraphRecall = true })
	for _, content := range []string{
		"alpha bravo charlie delta", "echo foxtrot golf hotel", "india juliet kilo lima",
		"the MCP server listens on port 8080 via SSE",
		"mike november oscar papa", "quebec romeo sierra tango",
	} {
		e.insertEntityFact(t, "CortexIQ", content, 0.8) // equal confidence, the answer in the middle
	}

	res, _, err := e.b.RecallDetailed(e.ctx, []string{e.ns}, "on which port does the CortexIQ MCP server listen", 10, RecallOptions{Types: []string{"fact"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Content == "the MCP server listens on port 8080 via SSE" {
			if r.Trace.GraphRank != 1 {
				t.Fatalf("the answer has graph rank %d, want 1 (closest to the question)", r.Trace.GraphRank)
			}
			return
		}
	}
	t.Fatalf("the answer was not returned: %+v", res)
}

// Failures and goals have no embedding: recall finds them by keyword, only when
// asked for by type, and names the right id field.
func TestIntegrationRecallFindsFailuresAndGoalsByType(t *testing.T) {
	e := newEnv(t, nil)
	e.insertFact(t, "the deploy pipeline uses blue green releases", nil)
	f, err := e.b.CreateFailure(e.ctx, e.nsID, "tried rolling restart of the deploy pipeline", "sessions dropped", "use blue green instead", nil)
	if err != nil {
		t.Fatal(err)
	}
	g, err := e.b.CreateGoal(e.ctx, e.nsID, "migrate the deploy pipeline to blue green", nil, 1)
	if err != nil {
		t.Fatal(err)
	}

	// Default recall must not change: no failures or goals.
	res, err := e.b.RecallWith(e.ctx, []string{e.ns}, "deploy pipeline", 10, RecallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Type == "failure" || r.Type == "goal" {
			t.Fatalf("default recall returned a %s", r.Type)
		}
	}

	res, err = e.b.RecallWith(e.ctx, []string{e.ns}, "deploy pipeline", 10, RecallOptions{Types: []string{"failure", "goal"}})
	if err != nil {
		t.Fatal(err)
	}
	var gotF, gotG bool
	for _, r := range res {
		switch r.Type {
		case "failure":
			gotF = r.FailureID != nil && *r.FailureID == f.ID && strings.Contains(r.Content, "lesson: use blue green instead")
		case "goal":
			gotG = r.GoalID != nil && *r.GoalID == g.ID && strings.Contains(r.Content, "status: active")
		case "fact", "episode":
			t.Fatalf("asked for failures and goals only, got %s", r.Type)
		}
	}
	if !gotF || !gotG {
		t.Fatalf("failure found %v, goal found %v: %+v", gotF, gotG, res)
	}

	// Mixed with facts they compete in one ranking and stay within the limit.
	res, err = e.b.RecallWith(e.ctx, []string{e.ns}, "deploy pipeline", 2, RecallOptions{Types: []string{"fact", "failure", "goal"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) > 2 {
		t.Fatalf("limit 2 returned %d results", len(res))
	}

	// A deleted failure disappears.
	if err := e.b.DeleteFailure(e.ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	res, _ = e.b.RecallWith(e.ctx, []string{e.ns}, "deploy pipeline", 10, RecallOptions{Types: []string{"failure"}})
	if len(res) != 0 {
		t.Fatalf("deleted failure still recalled: %+v", res)
	}
}

// Asking for facts and episodes together must not return facts only. Episodes
// used to receive whatever slots the facts left over, and facts fill the limit,
// so that branch never ran: a mixed request was a facts request with extra
// words. Each type now searches with its own budget and the merged list is
// sorted by score, so the better answer wins regardless of type.
func TestIntegrationRecallEpisodesCompeteWithFacts(t *testing.T) {
	e := newEnv(t, nil)
	for _, c := range []string{
		"alpha bravo charlie", "delta echo foxtrot", "golf hotel india",
		"juliet kilo lima", "mike november oscar",
	} {
		e.insertFact(t, c, nil)
	}
	ep, err := e.b.Remember(e.ctx, e.ns, "the nightly backup runs at 04:15", nil)
	if err != nil {
		t.Fatal(err)
	}

	res, err := e.b.RecallWith(e.ctx, []string{e.ns}, "when does the nightly backup run", 5, RecallOptions{Types: []string{"fact", "episode"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) > 5 {
		t.Fatalf("limit 5 returned %d results", len(res))
	}
	for _, r := range res {
		if r.Type == "episode" && r.EpisodeID != nil && *r.EpisodeID == ep {
			return
		}
	}
	t.Fatalf("the episode was crowded out by unrelated facts: %+v", res)
}

// With routing on, a query that points to failures finds them without naming a
// type; with routing off (the default) the same query does not.
func TestIntegrationRecallRoutesToFailures(t *testing.T) {
	e := newEnv(t, nil)
	e.insertFact(t, "the import job runs nightly", nil)
	if _, err := e.b.CreateFailure(e.ctx, e.nsID, "tried parallel import of the nightly job", "deadlocks", "keep the import sequential", nil); err != nil {
		t.Fatal(err)
	}
	q := "what did we try for the import job that failed"
	count := func(opts RecallOptions) (failures int, tr *RecallTrace) {
		res, tr, err := e.b.RecallDetailed(e.ctx, []string{e.ns}, q, 10, opts)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range res {
			if r.Type == "failure" {
				failures++
			}
		}
		return failures, tr
	}
	on, off := true, false

	if n, tr := count(RecallOptions{Route: &off}); n != 0 || len(tr.Routed) != 0 {
		t.Fatalf("routing off: %d failures, routed %v", n, tr.Routed)
	}
	if n, tr := count(RecallOptions{Route: &on}); n != 1 || len(tr.Routed) != 1 || tr.Routed[0] != "failure" {
		t.Fatalf("routing on: %d failures, routed %v", n, tr.Routed)
	}
	// Explicit types win over the router.
	if n, tr := count(RecallOptions{Route: &on, Types: []string{"fact"}}); n != 0 || len(tr.Routed) != 0 {
		t.Fatalf("explicit types: %d failures, routed %v", n, tr.Routed)
	}
}

// Reflect synthesizes an answer from recalled memories. The fake reasoner
// returns a canned answer, so this tests the recall grounding and result shape,
// not the LLM output itself.
func TestIntegrationReflect(t *testing.T) {
	e := newEnv(t, nil)
	e.insertFact(t, "the queue is kafka", nil)
	e.insertFact(t, "we switched from rabbitmq to kafka in march", nil)
	e.insertFact(t, "the database is postgres", nil)

	e.r.chatAnswer = "We use kafka for the queue, after switching from rabbitmq in March."

	result, err := e.b.Reflect(e.ctx, "what queue do we use?", ReflectOptions{
		Namespaces: []string{e.ns},
		Limit:      5,
	})
	if err != nil {
		t.Fatalf("Reflect: %v", err)
	}

	if result.Answer != e.r.chatAnswer {
		t.Errorf("Answer = %q, want %q", result.Answer, e.r.chatAnswer)
	}
	if result.RecallCount == 0 {
		t.Error("RecallCount = 0, want > 0: reflect should ground the answer in recalled memories")
	}
	if len(result.Sources) == 0 {
		t.Error("Sources is empty, want at least one source")
	}
	if result.DurationMs < 0 {
		t.Errorf("DurationMs = %d, want >= 0", result.DurationMs)
	}
	// The kafka facts should be among the sources.
	foundKafka := false
	for _, s := range result.Sources {
		if s.Type == "fact" && s.Content == "the queue is kafka" {
			foundKafka = true
		}
	}
	if !foundKafka {
		t.Errorf("sources do not include the kafka fact: %+v", result.Sources)
	}
}

// Reflect with no relevant memories still returns an answer (the LLM is told
// the memories are empty), and reports zero sources.
func TestIntegrationReflectNoMemories(t *testing.T) {
	e := newEnv(t, nil)
	e.r.chatAnswer = "I don't have any memory about that."

	// Global mental models are searched from every namespace by design, so this
	// test has to ask for raw memories only: it is about an empty namespace, not
	// about an empty memory store.
	result, err := e.b.Reflect(e.ctx, "what is the meaning of life?", ReflectOptions{
		Namespaces:       []string{e.ns},
		Limit:            5,
		SkipMentalModels: true,
	})
	if err != nil {
		t.Fatalf("Reflect: %v", err)
	}
	if result.Answer != e.r.chatAnswer {
		t.Errorf("Answer = %q, want %q", result.Answer, e.r.chatAnswer)
	}
	if len(result.Sources) != 0 {
		t.Errorf("Sources = %+v, want empty", result.Sources)
	}
}

// Reflect passes the additional context to the LLM prompt.
func TestIntegrationReflectWithContext(t *testing.T) {
	e := newEnv(t, nil)
	e.insertFact(t, "the project uses go", nil)

	_, err := e.b.Reflect(e.ctx, "what language?", ReflectOptions{
		Namespaces: []string{e.ns},
		Limit:      5,
		Context:    "The user is asking about the backend service.",
	})
	if err != nil {
		t.Fatalf("Reflect: %v", err)
	}
	if e.r.chatCalls != 1 {
		t.Fatalf("chatCalls = %d, want 1", e.r.chatCalls)
	}
}

// Reflect rejects an empty query before touching the database or the LLM.
func TestIntegrationReflectEmptyQuery(t *testing.T) {
	e := newEnv(t, nil)
	_, err := e.b.Reflect(e.ctx, "", ReflectOptions{Namespaces: []string{e.ns}})
	if err == nil {
		t.Fatal("Reflect with empty query should fail")
	}
	if e.r.chatCalls != 0 {
		t.Errorf("chatCalls = %d, want 0: no LLM call for an invalid query", e.r.chatCalls)
	}
}
