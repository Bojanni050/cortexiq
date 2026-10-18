package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pgvector/pgvector-go"
)

// Kinds of the graph question set (eval/graph_queries.jsonl).
const (
	// GraphQueryDirect names an entity but shares hardly any words with the fact
	// that answers it: the graph arm's job.
	GraphQueryDirect = "direct"
	// GraphQueryNeighbour asks about one entity; the answer is a fact about an
	// entity one relationship away.
	GraphQueryNeighbour = "neighbour"
	// GraphQueryControl names no entity. The arm has nothing to start from and
	// must change nothing.
	GraphQueryControl = "control"
	// GraphQueryDistractor names an entity with many facts, of which exactly one
	// answers: the arm must not bury it under the others.
	GraphQueryDistractor = "distractor"
)

// GraphFact is one fact of the graph corpus, with the entity it is about.
type GraphFact struct {
	ID      string `json:"id"`
	Entity  string `json:"entity"`
	Content string `json:"content"`
}

// GraphRel is one relationship between two entities of the corpus.
type GraphRel struct {
	From       string  `json:"from"`
	Type       string  `json:"type"`
	To         string  `json:"to"`
	Confidence float32 `json:"confidence"`
}

// graphLine is one line of eval/graph_corpus.jsonl: a fact or a relationship.
type graphLine struct {
	Kind string `json:"kind"` // "fact" or "rel"
	GraphFact
	GraphRel
}

// LoadGraphCorpus reads the corpus. Facts need an id, an entity and unique
// content; a relationship may only join entities that have a fact, so a typo in
// an entity name cannot silently disconnect the graph.
func LoadGraphCorpus(r io.Reader) ([]GraphFact, []GraphRel, error) {
	var facts []GraphFact
	var rels []GraphRel
	ids, contents, entities := map[string]bool{}, map[string]bool{}, map[string]bool{}
	err := readJSONL(r, func(line int, raw []byte) error {
		var l graphLine
		if err := json.Unmarshal(raw, &l); err != nil {
			return err
		}
		switch l.Kind {
		case "fact":
			f := l.GraphFact
			f.Content = strings.TrimSpace(f.Content)
			switch {
			case f.ID == "" || f.Entity == "" || f.Content == "":
				return fmt.Errorf("a fact needs id, entity and content")
			case ids[f.ID]:
				return fmt.Errorf("duplicate id %q", f.ID)
			case contents[f.Content]:
				return fmt.Errorf("%s repeats the content of another fact", f.ID)
			}
			ids[f.ID], contents[f.Content], entities[strings.ToLower(f.Entity)] = true, true, true
			facts = append(facts, f)
		case "rel":
			rel := l.GraphRel
			if rel.From == "" || rel.To == "" || rel.Type == "" {
				return fmt.Errorf("a relationship needs from, type and to")
			}
			if rel.Confidence <= 0 || rel.Confidence > 1 {
				rel.Confidence = 0.8
			}
			rels = append(rels, rel)
		default:
			return fmt.Errorf("kind %q is not fact or rel", l.Kind)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(facts) == 0 {
		return nil, nil, fmt.Errorf("no facts in the corpus")
	}
	for _, rel := range rels {
		for _, e := range []string{rel.From, rel.To} {
			if !entities[strings.ToLower(e)] {
				return nil, nil, fmt.Errorf("relationship %s -> %s names %q, which has no fact", rel.From, rel.To, e)
			}
		}
	}
	return facts, rels, nil
}

// LoadGraphQueries reads the question set and checks every expected fact exists.
func LoadGraphQueries(r io.Reader, facts []GraphFact) ([]RecallQuery, error) {
	known := map[string]bool{}
	for _, f := range facts {
		known[f.ID] = true
	}
	var qs []RecallQuery
	ids := map[string]bool{}
	err := readJSONL(r, func(line int, raw []byte) error {
		var q RecallQuery
		if err := json.Unmarshal(raw, &q); err != nil {
			return err
		}
		switch q.Kind {
		case GraphQueryDirect, GraphQueryNeighbour, GraphQueryControl, GraphQueryDistractor:
		default:
			return fmt.Errorf("%s: kind %q is not direct, neighbour, control or distractor", q.ID, q.Kind)
		}
		if q.ID == "" || strings.TrimSpace(q.Query) == "" || len(q.Expect) == 0 {
			return fmt.Errorf("id, query and expect are required")
		}
		if ids[q.ID] {
			return fmt.Errorf("duplicate id %q", q.ID)
		}
		ids[q.ID] = true
		for _, e := range q.Expect {
			if !known[e] {
				return fmt.Errorf("%s expects %q, which is not in the corpus", q.ID, e)
			}
		}
		qs = append(qs, q)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(qs) == 0 {
		return nil, fmt.Errorf("no questions in the set")
	}
	return qs, nil
}

// GraphEvalReport compares the same questions without and with the graph arm.
type GraphEvalReport struct {
	Model string `json:"model"`
	Limit int    `json:"limit"`
	Facts int    `json:"facts"`
	Rels  int    `json:"relationships"`
	// Without and With are the scores with the arm forced off and on.
	Without RecallEvalReport `json:"without"`
	With    RecallEvalReport `json:"with"`
	// Changes lists the questions whose rank differs, with both ranks, so a gain
	// or a regression can be read question by question.
	Changes []GraphChange `json:"changes,omitempty"`
}

// GraphChange is one question the arm moved.
type GraphChange struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Query       string `json:"query"`
	RankWithout int    `json:"rank_without"`
	RankWith    int    `json:"rank_with"`
}

// CompareGraphRuns pairs the outcomes of the two runs. It is pure.
func CompareGraphRuns(without, with RecallEvalReport) []GraphChange {
	byID := map[string]QueryOutcome{}
	for _, o := range with.Outcomes {
		byID[o.ID] = o
	}
	var out []GraphChange
	for _, o := range without.Outcomes {
		w := byID[o.ID]
		if w.Rank != o.Rank {
			out = append(out, GraphChange{ID: o.ID, Kind: o.Kind, Query: o.Query, RankWithout: o.Rank, RankWith: w.Rank})
		}
	}
	return out
}

// EvalGraph stores the corpus as facts and relationships in a temporary
// namespace under /eval/, asks every question twice through the normal recall
// path — with the graph arm forced off, then forced on — and removes the
// namespace again. The facts are written directly, with the configured
// embedder's vectors, because the point is to measure recall and not the
// extractor.
func (b *Brain) EvalGraph(ctx context.Context, facts []GraphFact, rels []GraphRel, qs []RecallQuery, opts RecallEvalOptions) (GraphEvalReport, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}
	slug := evalNamespacePrefix + "graph-" + time.Now().UTC().Format("20060102t150405")
	nsID, err := b.CreateNamespace(ctx, slug, "graph evaluation", "temporary namespace of 'eval graph'")
	if err != nil {
		return GraphEvalReport{}, err
	}
	if !opts.Keep {
		defer func() { _ = b.deleteEvalNamespace(context.WithoutCancel(ctx), slug) }()
	}

	byContent := map[string]string{}
	for _, f := range facts {
		vec, err := b.embedder.Embed(ctx, f.Content)
		if err != nil {
			return GraphEvalReport{}, fmt.Errorf("embed %s: %w", f.ID, err)
		}
		_, err = b.pool.Exec(ctx,
			`INSERT INTO facts (namespace_id, content, embedding, embedding_model, confidence, entity, valid_from)
			 VALUES ($1, $2, $3, $4, 0.8, $5, now())`,
			nsID, f.Content, pgvector.NewVector(vec), b.embedder.Model(), f.Entity)
		if err != nil {
			return GraphEvalReport{}, fmt.Errorf("store %s: %w", f.ID, err)
		}
		byContent[f.Content] = f.ID
	}
	for _, r := range rels {
		if _, err := b.pool.Exec(ctx,
			`INSERT INTO relationships (namespace_id, from_entity, relation_type, to_entity, confidence)
			 VALUES ($1, $2, $3, $4, $5)`,
			nsID, r.From, r.Type, r.To, r.Confidence); err != nil {
			return GraphEvalReport{}, fmt.Errorf("store relationship %s -> %s: %w", r.From, r.To, err)
		}
	}

	run := func(graph bool) (RecallEvalReport, error) {
		rankings := make([][]string, len(qs))
		for i, q := range qs {
			res, err := b.RecallWith(ctx, []string{slug}, q.Query, limit, RecallOptions{Types: []string{"fact"}, Graph: &graph})
			if err != nil {
				return RecallEvalReport{}, fmt.Errorf("recall %s: %w", q.ID, err)
			}
			for _, r := range res {
				if id, ok := byContent[strings.TrimSpace(r.Content)]; ok {
					rankings[i] = append(rankings[i], id)
				}
			}
		}
		rep := ScoreRecall(qs, rankings, limit)
		rep.Model, rep.Docs = b.embedder.Model(), len(facts)
		return rep, nil
	}

	without, err := run(false)
	if err != nil {
		return GraphEvalReport{}, err
	}
	with, err := run(true)
	if err != nil {
		return GraphEvalReport{}, err
	}
	return GraphEvalReport{
		Model: b.embedder.Model(), Limit: limit, Facts: len(facts), Rels: len(rels),
		Without: without, With: with, Changes: CompareGraphRuns(without, with),
	}, nil
}
