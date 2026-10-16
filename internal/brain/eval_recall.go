package brain

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// Kinds of the recall question set (eval/recall_queries.jsonl).
const (
	// QueryIdentifier is an exact token (env var, file, metric): the keyword
	// ranker's job.
	QueryIdentifier = "identifier"
	// QueryParaphrase shares meaning with its answer but hardly any words: the
	// vector ranker's job.
	QueryParaphrase = "paraphrase"
	// QueryCrossLanguage asks in Dutch about an English memory or the reverse.
	QueryCrossLanguage = "cross-language"
	// QueryConfuser has a near-twin in the corpus that must not outrank the answer.
	QueryConfuser = "confuser"
)

// RecallDoc is one memory of the recall corpus. It is stored as an episode.
type RecallDoc struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// RecallQuery is one question with the document(s) that answer it.
type RecallQuery struct {
	ID     string   `json:"id"`
	Kind   string   `json:"kind"`
	Query  string   `json:"query"`
	Expect []string `json:"expect"`
}

// LoadRecallDocs reads the corpus. IDs and contents must be unique: a result is
// mapped back to its document by content.
func LoadRecallDocs(r io.Reader) ([]RecallDoc, error) {
	var docs []RecallDoc
	ids, contents := map[string]bool{}, map[string]bool{}
	err := readJSONL(r, func(line int, raw []byte) error {
		var d RecallDoc
		if err := json.Unmarshal(raw, &d); err != nil {
			return err
		}
		d.Content = strings.TrimSpace(d.Content)
		switch {
		case d.ID == "" || d.Content == "":
			return fmt.Errorf("id and content are required")
		case ids[d.ID]:
			return fmt.Errorf("duplicate id %q", d.ID)
		case contents[d.Content]:
			return fmt.Errorf("%s repeats the content of another document", d.ID)
		}
		ids[d.ID], contents[d.Content] = true, true
		docs = append(docs, d)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no documents in the corpus")
	}
	return docs, nil
}

// LoadRecallQueries reads the question set and checks every expected document
// exists in docs, so a typo cannot turn into a silent miss.
func LoadRecallQueries(r io.Reader, docs []RecallDoc) ([]RecallQuery, error) {
	known := map[string]bool{}
	for _, d := range docs {
		known[d.ID] = true
	}
	var qs []RecallQuery
	ids := map[string]bool{}
	err := readJSONL(r, func(line int, raw []byte) error {
		var q RecallQuery
		if err := json.Unmarshal(raw, &q); err != nil {
			return err
		}
		switch q.Kind {
		case QueryIdentifier, QueryParaphrase, QueryCrossLanguage, QueryConfuser:
		default:
			return fmt.Errorf("%s: kind %q is not identifier, paraphrase, cross-language or confuser", q.ID, q.Kind)
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

// readJSONL calls fn for every non-blank line and names the line in its errors.
func readJSONL(r io.Reader, fn func(line int, raw []byte) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		if err := fn(line, []byte(text)); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
	}
	return sc.Err()
}

// QueryOutcome is how one question did.
type QueryOutcome struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Query string `json:"query"`
	// Rank is the 1-based position of the first expected document, 0 when it did
	// not come back at all.
	Rank int `json:"rank"`
	// Top lists the first documents returned, best first.
	Top []string `json:"top"`
}

// RecallStats are the scores of one group of questions.
type RecallStats struct {
	Questions int     `json:"questions"`
	HitAt1    float64 `json:"hit_at_1"`
	HitAt3    float64 `json:"hit_at_3"`
	HitAtK    float64 `json:"hit_at_k"`
	MRR       float64 `json:"mrr"`
}

// RecallEvalReport is the result of scoring a question set.
type RecallEvalReport struct {
	Model string `json:"model"`
	// Limit is k: how many results each question asked for.
	Limit   int                    `json:"limit"`
	Docs    int                    `json:"docs"`
	Overall RecallStats            `json:"overall"`
	Kinds   map[string]RecallStats `json:"kinds"`
	// Misses are the questions whose answer was not first, worst first.
	Misses   []QueryOutcome `json:"misses,omitempty"`
	Outcomes []QueryOutcome `json:"outcomes"`
}

// ScoreRecall turns ranked document IDs into a report. rankings[i] holds the
// documents returned for qs[i], best first. It is pure: seeding and recalling
// happen in EvalRecall.
func ScoreRecall(qs []RecallQuery, rankings [][]string, limit int) RecallEvalReport {
	rep := RecallEvalReport{Limit: limit, Kinds: map[string]RecallStats{}}
	byKind := map[string][]QueryOutcome{}
	for i, q := range qs {
		want := map[string]bool{}
		for _, e := range q.Expect {
			want[e] = true
		}
		o := QueryOutcome{ID: q.ID, Kind: q.Kind, Query: q.Query}
		for pos, id := range rankings[i] {
			if o.Rank == 0 && want[id] {
				o.Rank = pos + 1
			}
		}
		o.Top = rankings[i]
		if len(o.Top) > 3 {
			o.Top = o.Top[:3]
		}
		rep.Outcomes = append(rep.Outcomes, o)
		byKind[q.Kind] = append(byKind[q.Kind], o)
		if o.Rank != 1 {
			rep.Misses = append(rep.Misses, o)
		}
	}
	rep.Overall = recallStats(rep.Outcomes, limit)
	for kind, outs := range byKind {
		rep.Kinds[kind] = recallStats(outs, limit)
	}
	sort.SliceStable(rep.Misses, func(i, j int) bool {
		a, b := rep.Misses[i].Rank, rep.Misses[j].Rank
		if a == 0 || b == 0 {
			return a == 0 && b != 0
		}
		return a > b
	})
	return rep
}

func recallStats(outs []QueryOutcome, limit int) RecallStats {
	s := RecallStats{Questions: len(outs)}
	if len(outs) == 0 {
		return s
	}
	for _, o := range outs {
		if o.Rank == 0 {
			continue
		}
		s.MRR += 1 / float64(o.Rank)
		if o.Rank <= 1 {
			s.HitAt1++
		}
		if o.Rank <= 3 {
			s.HitAt3++
		}
		if o.Rank <= limit {
			s.HitAtK++
		}
	}
	n := float64(len(outs))
	s.HitAt1 /= n
	s.HitAt3 /= n
	s.HitAtK /= n
	s.MRR /= n
	return s
}

// RecallEvalOptions controls EvalRecall.
type RecallEvalOptions struct {
	// Limit is how many results each question asks for (default 10).
	Limit int
	// Keep leaves the temporary namespace in place, to inspect it by hand.
	Keep bool
}

// evalNamespacePrefix is the only place EvalRecall writes to, and the only
// place it deletes from.
const evalNamespacePrefix = "/eval/"

// EvalRecall stores the corpus as episodes in a temporary namespace under /eval/,
// asks every question through the normal recall path (the configured embedder,
// hybrid ranking and weights), and removes the namespace again. Only episode
// results are ranked: the corpus has no facts, but a background consolidation of
// the namespace could create some, and those must not take a rank from a document.
func (b *Brain) EvalRecall(ctx context.Context, docs []RecallDoc, qs []RecallQuery, opts RecallEvalOptions) (RecallEvalReport, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}
	slug := evalNamespacePrefix + "recall-" + time.Now().UTC().Format("20060102t150405")
	if _, err := b.CreateNamespace(ctx, slug, "recall evaluation", "temporary namespace of 'eval recall'"); err != nil {
		return RecallEvalReport{}, err
	}
	if !opts.Keep {
		// A fresh context: the run may have been cancelled, the cleanup must not be.
		defer func() { _ = b.deleteEvalNamespace(context.WithoutCancel(ctx), slug) }()
	}

	byContent := map[string]string{}
	for _, d := range docs {
		if _, err := b.Remember(ctx, slug, d.Content, nil); err != nil {
			return RecallEvalReport{}, fmt.Errorf("store %s: %w", d.ID, err)
		}
		byContent[d.Content] = d.ID
	}

	rankings := make([][]string, len(qs))
	for i, q := range qs {
		res, err := b.Recall(ctx, []string{slug}, q.Query, limit)
		if err != nil {
			return RecallEvalReport{}, fmt.Errorf("recall %s: %w", q.ID, err)
		}
		for _, r := range res {
			if r.Type != RecordEpisode {
				continue
			}
			if id, ok := byContent[strings.TrimSpace(r.Content)]; ok {
				rankings[i] = append(rankings[i], id)
			}
		}
	}

	rep := ScoreRecall(qs, rankings, limit)
	rep.Model = b.embedder.Model()
	rep.Docs = len(docs)
	return rep, nil
}

// deleteEvalNamespace removes a namespace created by EvalRecall; its episodes go
// with it (ON DELETE CASCADE). It refuses any slug outside /eval/, so a bug here
// can never touch real memories.
func (b *Brain) deleteEvalNamespace(ctx context.Context, slug string) error {
	if !strings.HasPrefix(slug, evalNamespacePrefix) || len(slug) <= len(evalNamespacePrefix) {
		return fmt.Errorf("refusing to delete namespace %q outside %s", slug, evalNamespacePrefix)
	}
	_, err := b.pool.Exec(ctx, "DELETE FROM namespaces WHERE slug = $1", slug)
	return err
}
