//go:build modeleval

// Model evaluation harness: runs every consolidation stage of the real reasoner
// (production prompts, parsing, grounding and retry) against one model, so a model
// can be compared with the one in production before it is switched. It calls an
// external API and costs money, so it only builds with -tags modeleval and is never
// part of `go test ./...`.
//
//	EVAL_BASE_URL=https://api.deepinfra.com/v1/openai EVAL_MODEL=google/gemma-4-31B-it \
//	EVAL_KEY_ENV=DEEPINFRA_API_KEY EVAL_INPUTS=stage_inputs.json EVAL_OUT=out \
//	go test -tags modeleval -run TestModelEval -count=1 -timeout 60m ./internal/reasoner/
//
// EVAL_INPUTS is a JSON file with episodes, facts, relationships and contradictions
// (see the scratchpad query that produced it); the labelled dedup pairs come from
// eval/dedup_pairs.jsonl. Nothing is written to a database.
package reasoner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
)

type evalInputs struct {
	Episodes []struct {
		ID      int64  `json:"id"`
		Subject string `json:"subject"`
		Content string `json:"content"`
	} `json:"episodes"`
	Facts []struct {
		ID      int64  `json:"id"`
		Content string `json:"content"`
	} `json:"facts"`
	Relationships []struct {
		ID         int64   `json:"id"`
		FactID     int64   `json:"fact_id"`
		From       string  `json:"from_entity"`
		Type       string  `json:"relation_type"`
		To         string  `json:"to_entity"`
		Confidence float32 `json:"confidence"`
	} `json:"relationships"`
	Contradictions []struct {
		Entity   string `json:"entity"`
		Property string `json:"property"`
		Old      string `json:"old_value"`
		New      string `json:"new_value"`
	} `json:"contradictions"`
}

type stageResult struct {
	Stage   string          `json:"stage"`
	Calls   int             `json:"calls"`
	Errors  int             `json:"errors"`
	Seconds float64         `json:"seconds_total"`
	Slowest float64         `json:"seconds_slowest"`
	Detail  map[string]any  `json:"detail,omitempty"`
	Errs    []string        `json:"error_samples,omitempty"`
	Sample  json.RawMessage `json:"sample,omitempty"`
}

// runStage executes n calls with a small worker pool and aggregates timing and errors.
func runStage(name string, n int, fn func(i int) (any, error)) (stageResult, []any) {
	res := stageResult{Stage: name, Calls: n, Detail: map[string]any{}}
	outs := make([]any, n)
	var mu sync.Mutex
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			t0 := time.Now()
			out, err := fn(i)
			d := time.Since(t0).Seconds()
			mu.Lock()
			defer mu.Unlock()
			if d > res.Slowest {
				res.Slowest = d
			}
			if err != nil {
				res.Errors++
				if len(res.Errs) < 3 {
					msg := err.Error()
					if len(msg) > 160 {
						msg = msg[:160]
					}
					res.Errs = append(res.Errs, msg)
				}
				return
			}
			outs[i] = out
		}(i)
	}
	wg.Wait()
	res.Seconds = time.Since(start).Seconds()
	return res, outs
}

func TestModelEval(t *testing.T) {
	baseURL, model := os.Getenv("EVAL_BASE_URL"), os.Getenv("EVAL_MODEL")
	keyEnv := os.Getenv("EVAL_KEY_ENV")
	if keyEnv == "" {
		keyEnv = "DEEPINFRA_API_KEY"
	}
	key := os.Getenv(keyEnv)
	if baseURL == "" || model == "" || key == "" {
		t.Skipf("set EVAL_BASE_URL, EVAL_MODEL and the key variable %s", keyEnv)
	}
	maxTok := 1024
	if v := os.Getenv("EVAL_MAXTOK"); v != "" {
		maxTok, _ = strconv.Atoi(v)
	}
	jsonMode := os.Getenv("EVAL_JSON_MODE") == "true"
	outDir := os.Getenv("EVAL_OUT")
	if outDir == "" {
		outDir = t.TempDir()
	}
	_ = os.MkdirAll(outDir, 0o755)

	raw, err := os.ReadFile(os.Getenv("EVAL_INPUTS"))
	if err != nil {
		t.Fatalf("read EVAL_INPUTS: %v", err)
	}
	var in evalInputs
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatalf("parse inputs: %v", err)
	}

	r, err := NewOpenAI(baseURL, key, model, maxTok, jsonMode)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var results []stageResult

	// 1. facts from episodes
	{
		res, outs := runStage("structured (episode -> facts)", len(in.Episodes), func(i int) (any, error) {
			e := in.Episodes[i]
			fs, err := r.ReasonStructured(ctx, []string{e.Content}, e.Subject)
			if err != nil {
				return nil, err
			}
			return fs, nil
		})
		total, empty := 0, 0
		var sample []map[string]any
		for i, o := range outs {
			fs, _ := o.([]*StructuredFact)
			if o == nil {
				continue
			}
			total += len(fs)
			if len(fs) == 0 {
				empty++
			}
			if i < 4 {
				var sums []string
				for _, f := range fs {
					sums = append(sums, f.Summary)
				}
				sample = append(sample, map[string]any{"episode": in.Episodes[i].Content, "facts": sums})
			}
		}
		res.Detail["facts_total"] = total
		res.Detail["episodes_without_facts"] = empty
		res.Sample, _ = json.Marshal(sample)
		results = append(results, res)
	}

	// 2. same-fact review, scored against the labelled pairs
	{
		f, err := os.Open("../../eval/dedup_pairs.jsonl")
		if err != nil {
			t.Fatalf("dedup pairs: %v", err)
		}
		var pairs []struct{ ID, Label, A, B string }
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var p struct {
				ID    string `json:"id"`
				Label string `json:"label"`
				A     string `json:"a"`
				B     string `json:"b"`
			}
			if err := json.Unmarshal([]byte(line), &p); err != nil {
				t.Fatal(err)
			}
			pairs = append(pairs, struct{ ID, Label, A, B string }{p.ID, p.Label, p.A, p.B})
		}
		f.Close()
		res, outs := runStage("same-fact review (labelled pairs)", len(pairs), func(i int) (any, error) {
			return r.ReasonSameFact(ctx, pairs[i].A, pairs[i].B)
		})
		sameOK, sameN, wrongMerge, otherN := 0, 0, 0, 0
		var wrong []string
		for i, o := range outs {
			if o == nil {
				continue
			}
			same := o.(bool)
			if pairs[i].Label == "same" {
				sameN++
				if same {
					sameOK++
				}
			} else {
				otherN++
				if same {
					wrongMerge++
					wrong = append(wrong, pairs[i].ID)
				}
			}
		}
		res.Detail["twins_merged"] = fmt.Sprintf("%d/%d", sameOK, sameN)
		res.Detail["others_wrongly_merged"] = fmt.Sprintf("%d/%d", wrongMerge, otherN)
		res.Detail["wrong_merge_ids"] = wrong
		results = append(results, res)
	}

	// 3. relationships, batches of 5 like production
	facts := make([]models.Fact, len(in.Facts))
	for i, f := range in.Facts {
		facts[i] = models.Fact{ID: f.ID, Content: f.Content}
	}
	{
		const batch = 5
		nb := (len(facts) + batch - 1) / batch
		res, outs := runStage("relationships (5 facts per call)", nb, func(i int) (any, error) {
			end := (i + 1) * batch
			if end > len(facts) {
				end = len(facts)
			}
			return r.ReasonRelationshipsBatch(ctx, facts[i*batch:end])
		})
		rels, mapped := 0, 0
		for _, o := range outs {
			m, _ := o.(map[int64][]*StructuredRelationship)
			for _, list := range m {
				for _, x := range list {
					rels++
					if x.CanonicalType != "" {
						mapped++
					}
				}
			}
		}
		res.Detail["relations"] = rels
		res.Detail["with_canonical_type"] = mapped
		results = append(results, res)
	}

	// 4. patterns, 20 facts + their relationships per call
	{
		const batch = 20
		byFact := map[int64][]models.Relationship{}
		for _, x := range in.Relationships {
			byFact[x.FactID] = append(byFact[x.FactID], models.Relationship{ID: x.ID, FromEntity: x.From, RelationType: x.Type, ToEntity: x.To, Confidence: x.Confidence})
		}
		nb := (len(facts) + batch - 1) / batch
		res, outs := runStage("patterns (20 facts per call)", nb, func(i int) (any, error) {
			end := (i + 1) * batch
			if end > len(facts) {
				end = len(facts)
			}
			var rs []models.Relationship
			for _, f := range facts[i*batch : end] {
				rs = append(rs, byFact[f.ID]...)
			}
			return r.ReasonPatterns(ctx, facts[i*batch:end], rs)
		})
		n := 0
		for _, o := range outs {
			ps, _ := o.([]*StructuredPattern)
			n += len(ps)
		}
		res.Detail["patterns"] = n
		results = append(results, res)
	}

	// 5. contradictions
	if len(in.Contradictions) > 0 {
		res, outs := runStage("contradiction", len(in.Contradictions), func(i int) (any, error) {
			c := in.Contradictions[i]
			return r.ReasonContradiction(ctx, c.Entity, c.Property, c.Old, c.New)
		})
		counts := map[string]int{}
		for _, o := range outs {
			if cr, ok := o.(*ContradictionResult); ok && cr != nil {
				counts[string(cr.Classification)]++
			}
		}
		res.Detail["classifications"] = counts
		results = append(results, res)
	}

	// 6. causal links, 10 facts per call
	{
		const batch = 10
		nb := (len(facts) + batch - 1) / batch
		res, outs := runStage("causal links (10 facts per call)", nb, func(i int) (any, error) {
			end := (i + 1) * batch
			if end > len(facts) {
				end = len(facts)
			}
			return r.ReasonCausalLinks(ctx, facts[i*batch:end])
		})
		n := 0
		for _, o := range outs {
			ls, _ := o.([]*StructuredCausalLink)
			n += len(ls)
		}
		res.Detail["links"] = n
		results = append(results, res)
	}

	b, _ := json.MarshalIndent(map[string]any{"model": model, "max_tokens": maxTok, "json_mode": jsonMode, "stages": results}, "", "  ")
	file := filepath.Join(outDir, strings.ReplaceAll(model, "/", "__")+".json")
	if err := os.WriteFile(file, b, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("model %s (max_tokens %d, json_mode %v)", model, maxTok, jsonMode)
	for _, s := range results {
		t.Logf("%-36s calls %3d  errors %3d  total %6.0fs  slowest %5.1fs  %v", s.Stage, s.Calls, s.Errors, s.Seconds, s.Slowest, s.Detail)
	}
}
