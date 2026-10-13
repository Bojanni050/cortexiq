package brain

// Integration tests against a real Postgres (with pgvector). They are skipped
// unless STASH_TEST_DSN is set, e.g.
//
//	STASH_TEST_DSN=postgres://stash:stash@localhost:5432/stash_test?sslmode=disable go test ./internal/brain/
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

	"github.com/alash3al/stash/internal/db"
	"github.com/alash3al/stash/internal/models"
	"github.com/alash3al/stash/internal/queries"
	"github.com/alash3al/stash/internal/reasoner"
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
}

func (r *fakeReasoner) ReasonStructured(_ context.Context, texts []string) (*reasoner.StructuredFact, error) {
	return &reasoner.StructuredFact{Summary: texts[0]}, nil
}
func (r *fakeReasoner) ReasonRelationshipsBatch(context.Context, []models.Fact) (map[int64][]*reasoner.StructuredRelationship, error) {
	return nil, nil
}
func (r *fakeReasoner) ReasonPatterns(context.Context, []models.Fact, []models.Relationship) ([]*reasoner.StructuredPattern, error) {
	return nil, nil
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
	dsn := os.Getenv("STASH_TEST_DSN")
	if dsn == "" {
		t.Skip("STASH_TEST_DSN not set; skipping integration test")
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
	live := e.insertFact(t, "stash dashboard runs on port 8445", nil)
	expired := e.insertFact(t, "stash dashboard runs on port 9999", &past)

	res, err := e.b.Recall(e.ctx, []string{e.ns}, "stash dashboard port", 10)
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
	text := "the stash server listens on port 8080"

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

	if err := e.b.PurgeEpisode(e.ctx, epID); err != nil {
		t.Fatal(err)
	}
	if err := e.b.PurgeFact(e.ctx, factID); err != nil {
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
