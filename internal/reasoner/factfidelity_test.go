//go:build modeleval

// Fact fidelity harness: does a fact still say what its source episode says?
//
// It runs every episode through two arms against one model and writes a side by
// side report for a human to read, plus numbers that need no judge:
//
//	A  production: the real ReasonStructured (prompt rule "rewrite in your own words"
//	   and the 80 percent word-overlap grounding gate that drops close copies).
//	B  variant: the same prompt with that one rule replaced by "keep the wording where
//	   it is correct and self-contained, never swap subject and object", and no gate.
//
// Nothing here touches production code or a database. It calls an external API and
// costs money, so it only builds with -tags modeleval.
//
//	EVAL_BASE_URL=... EVAL_MODEL=... EVAL_KEY_ENV=... EVAL_INPUTS=stage_inputs.json \
//	EVAL_LIMIT=20 EVAL_OUT=out \
//	go test -tags modeleval -run TestFactFidelity -count=1 -timeout 30m ./internal/reasoner/
//
// EVAL_INPUTS is the same file TestModelEval reads (only its "episodes" are used).
package reasoner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/openai/openai-go"
)

const fidelityOldRule = "- Every summary MUST be specific and detailed. Do NOT echo, quote, or closely paraphrase a single Event sentence; rewrite in your own words."

const fidelityNewRule = "- Every summary MUST be specific and detailed. Keep the wording of the Events where it is correct and already self-contained. Rewrite only to remove first-person markers, pronouns such as \"it\" or \"this\", and temporary states. Keep subject and object exactly as the Events state them: never swap them."

// fidelityNumbers matches tokens that carry a digit: versions, ports, dates, counts.
var fidelityNumbers = regexp.MustCompile(`[0-9][0-9A-Za-z.\-:/]*[0-9A-Za-z]|[0-9]`)

func fidelityTokens(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range fidelityNumbers.FindAllString(strings.ToLower(s), -1) {
		m = strings.Trim(m, ".-:/")
		if m != "" && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

type fidelityArm struct {
	Facts       []string `json:"facts"`
	Err         string   `json:"error,omitempty"`
	Invented    []string `json:"invented_numbers,omitempty"`
	Retained    int      `json:"episode_numbers_kept"`
	OfTotal     int      `json:"episode_numbers_total"`
	GateRejects int      `json:"would_be_rejected_by_gate"`
}

type fidelityRow struct {
	ID      int64       `json:"episode_id"`
	Episode string      `json:"episode"`
	A       fidelityArm `json:"a_production"`
	B       fidelityArm `json:"b_variant"`
}

func scoreFidelityArm(episode string, facts []string, err error, gate func(string) error) fidelityArm {
	arm := fidelityArm{Facts: facts}
	if err != nil {
		arm.Err = err.Error()
		return arm
	}
	epTokens := fidelityTokens(episode)
	arm.OfTotal = len(epTokens)
	joined := strings.ToLower(strings.Join(facts, "\n"))
	for _, t := range epTokens {
		if strings.Contains(joined, t) {
			arm.Retained++
		}
	}
	epLower := strings.ToLower(episode)
	for _, f := range facts {
		for _, t := range fidelityTokens(f) {
			if !strings.Contains(epLower, t) {
				arm.Invented = append(arm.Invented, t)
			}
		}
		if gate != nil && gate(f) != nil {
			arm.GateRejects++
		}
	}
	return arm
}

func TestFactFidelity(t *testing.T) {
	baseURL, model := os.Getenv("EVAL_BASE_URL"), os.Getenv("EVAL_MODEL")
	keyEnv := os.Getenv("EVAL_KEY_ENV")
	if keyEnv == "" {
		keyEnv = "DEEPINFRA_API_KEY"
	}
	key := os.Getenv(keyEnv)
	if baseURL == "" || model == "" || key == "" {
		t.Skipf("set EVAL_BASE_URL, EVAL_MODEL and the key variable %s", keyEnv)
	}
	maxTok := 8192
	if v := os.Getenv("EVAL_MAXTOK"); v != "" {
		maxTok, _ = strconv.Atoi(v)
	}
	limit := 20
	if v := os.Getenv("EVAL_LIMIT"); v != "" {
		limit, _ = strconv.Atoi(v)
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
	if len(in.Episodes) > limit {
		in.Episodes = in.Episodes[:limit]
	}

	// The variant is the production prompt with exactly one rule swapped. If the
	// production prompt changes and the rule is no longer found, fail loudly instead
	// of comparing a prompt with itself.
	if !strings.Contains(structuredPrompt([]string{"x"}, ""), fidelityOldRule) {
		t.Fatal("production prompt no longer contains the rule this harness replaces; update fidelityOldRule")
	}

	r, err := NewOpenAI(baseURL, key, model, maxTok, jsonMode)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	variant := func(e struct {
		ID      int64  `json:"id"`
		Subject string `json:"subject"`
		Content string `json:"content"`
	}) ([]string, error) {
		prompt := strings.Replace(structuredPrompt([]string{e.Content}, e.Subject), fidelityOldRule, fidelityNewRule, 1)
		msgs := []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(prompt),
		}
		out, err := r.chat(ctx, "structured", msgs)
		if err != nil {
			return nil, err
		}
		fs, err := parseStructuredFacts(out)
		if err != nil {
			return nil, err
		}
		var sums []string
		for _, f := range fs {
			sums = append(sums, f.Summary)
		}
		return sums, nil
	}

	rows := make([]fidelityRow, 0, len(in.Episodes))
	for _, e := range in.Episodes {
		row := fidelityRow{ID: e.ID, Episode: e.Content}
		gate := func(s string) error {
			return validateFactGrounding(&StructuredFact{Summary: s}, []string{e.Content})
		}

		fa, errA := r.ReasonStructured(ctx, []string{e.Content}, e.Subject)
		var sa []string
		for _, f := range fa {
			sa = append(sa, f.Summary)
		}
		row.A = scoreFidelityArm(e.Content, sa, errA, nil)

		sb, errB := variant(e)
		row.B = scoreFidelityArm(e.Content, sb, errB, gate)
		rows = append(rows, row)
	}

	var md strings.Builder
	fmt.Fprintf(&md, "# Fact fidelity: %s\n\nA = production prompt and gate. B = rule replaced, no gate. Read the facts against the episode: subject and object right? detail kept? anything added?\n\n", model)
	var aKept, aTot, bKept, bTot, aInv, bInv, bGate, aN, bN int
	for _, row := range rows {
		fmt.Fprintf(&md, "## Episode %d\n\n> %s\n\n", row.ID, strings.ReplaceAll(strings.TrimSpace(row.Episode), "\n", "\n> "))
		for _, side := range []struct {
			name string
			arm  fidelityArm
		}{{"A production", row.A}, {"B variant", row.B}} {
			fmt.Fprintf(&md, "**%s** (numbers kept %d/%d, invented %v, gate would reject %d)\n\n", side.name, side.arm.Retained, side.arm.OfTotal, side.arm.Invented, side.arm.GateRejects)
			if side.arm.Err != "" {
				fmt.Fprintf(&md, "- ERROR: %s\n", side.arm.Err)
			}
			for _, f := range side.arm.Facts {
				fmt.Fprintf(&md, "- %s\n", f)
			}
			md.WriteString("\n")
		}
		aKept += row.A.Retained
		aTot += row.A.OfTotal
		bKept += row.B.Retained
		bTot += row.B.OfTotal
		aInv += len(row.A.Invented)
		bInv += len(row.B.Invented)
		bGate += row.B.GateRejects
		aN += len(row.A.Facts)
		bN += len(row.B.Facts)
	}
	summary := fmt.Sprintf("episodes %d | A: %d facts, numbers kept %d/%d, invented %d | B: %d facts, numbers kept %d/%d, invented %d, would be rejected by the production gate %d",
		len(rows), aN, aKept, aTot, aInv, bN, bKept, bTot, bInv, bGate)
	fmt.Fprintf(&md, "---\n\n%s\n", summary)

	base := filepath.Join(outDir, "fidelity_"+strings.ReplaceAll(model, "/", "__"))
	if err := os.WriteFile(base+".md", []byte(md.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	js, _ := json.MarshalIndent(map[string]any{"model": model, "max_tokens": maxTok, "json_mode": jsonMode, "rows": rows}, "", "  ")
	if err := os.WriteFile(base+".json", js, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Log(summary)
	t.Logf("report: %s.md", base)
}
