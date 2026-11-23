// Modifications copyright (c) 2026 Bojan van der Heide
package reasoner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// blockServer answers every completion with the given content, JSON encoded as
// it is. Content blocks are a list here, exactly like the provider sends them:
// the SDK reads them into the content string loosely, which is the behaviour the
// flattening has to survive.
func blockServer(t *testing.T, content any, finishReason string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s, want /chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1,
			"model":   "reasoning",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": content},
				"finish_reason": finishReason,
			}},
			"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// thinkingBlock is what a provider sends for the trace; reasoning models put one
// in front of the answer.
func thinkingBlock(text string) map[string]any {
	return map[string]any{
		"type":     "thinking",
		"thinking": []map[string]any{{"type": "text", "text": text}},
	}
}

func textBlock(text string) map[string]any {
	return map[string]any{"type": "text", "text": text}
}

func TestFlattenContentBlocksKeepsTheAnswerAndDropsTheTrace(t *testing.T) {
	content := `[{"type":"thinking","thinking":[{"type":"text","text":"Let me analyze this candidate."}]},
	             {"type":"text","text":"[{\"candidate\":1,\"statement\":\"every service runs on the same host\",\"coherence\":1.0}]"}]`

	text, reasoningOnly, ok := flattenContentBlocks(content)
	if !ok {
		t.Fatal("a block list must be recognised as blocks")
	}
	if reasoningOnly {
		t.Error("an answer with a text block is not a reasoning-only answer")
	}
	if !strings.HasPrefix(text, `[{"candidate":1`) {
		t.Errorf("the text block must come through as the answer, got %q", text)
	}
	if strings.Contains(text, "Let me analyze") {
		t.Errorf("the reasoning trace must not be part of the answer: %q", text)
	}
}

func TestFlattenContentBlocksJoinsSeveralTextBlocks(t *testing.T) {
	content := `[{"type":"text","text":"[{\"candidate\":1,"},{"type":"text","text":"\"statement\":\"x\"}]"}]`

	text, _, ok := flattenContentBlocks(content)
	if !ok {
		t.Fatal("want blocks")
	}
	if text != `[{"candidate":1,"statement":"x"}]` {
		t.Errorf("text blocks must be joined, got %q", text)
	}
}

// Production, 2026-11-23: the shape sweep failed on "unreadable answer to 1
// shape candidates: [{\"type\":\"thinking\"…". The model had answered — the
// trace was just in the way.
func TestReasonShapesReadsAContentBlockAnswer(t *testing.T) {
	answer := `[{"candidate": 1, "statement": "every client reaches its server over the http-first transport", "coherence": 1.0}]`
	srv := blockServer(t, []any{
		thinkingBlock("Let me analyze this candidate.\n\nThe relation is uses_transport."),
		textBlock(answer),
	}, "stop")
	r := newTriage(t, srv.URL)

	got, err := r.ReasonShapes(context.Background(), shapeCandidates()[:1])
	if err != nil {
		t.Fatalf("ReasonShapes: %v", err)
	}
	if len(got) != 1 || got[0].Statement == "" {
		t.Fatalf("the block answer was not read: %+v", got)
	}
	if got[0].Coherence != 1.0 {
		t.Errorf("coherence lost: %+v", got[0])
	}
}

func TestFlattenContentBlocksReportsAReasoningOnlyAnswer(t *testing.T) {
	content := `[{"type":"thinking","thinking":[{"type":"text","text":"Let me analyze this candidate."}]}]`

	text, reasoningOnly, ok := flattenContentBlocks(content)
	if !ok || !reasoningOnly {
		t.Fatalf("want a reasoning-only block list, got ok=%v reasoningOnly=%v", ok, reasoningOnly)
	}
	if text != "" {
		t.Errorf("a trace is not an answer, got %q", text)
	}
}

// A block list that ran into the output limit is still a trace, so it must be
// reported as such instead of as an unreadable JSON array.
func TestFlattenContentBlocksSalvagesACutOffTrace(t *testing.T) {
	content := `[{"type":"thinking","thinking":[{"type":"text","text":"Let me analyze"},`

	_, reasoningOnly, ok := flattenContentBlocks(content)
	if !ok || !reasoningOnly {
		t.Fatalf("a cut-off trace must still count as blocks, got ok=%v reasoningOnly=%v", ok, reasoningOnly)
	}
}

// The failure stage answers with objects that carry a "type" field of their own.
// Reading those as content blocks would erase a valid answer.
func TestFlattenContentBlocksLeavesAnAnswerArrayAlone(t *testing.T) {
	for _, content := range []string{
		`[{"type":"repeated_failure","failure_id":3,"evidence":"x","pattern_fact":"y","confidence":0.8}]`,
		`[{"candidate":1,"statement":"x","coherence":1.0}]`,
		`[{"entity":"CortexIQ","property":"port","value":"18101"}]`,
	} {
		if _, _, ok := flattenContentBlocks(content); ok {
			t.Errorf("an answer array was taken for content blocks: %s", content)
		}
	}
}

func TestFlattenContentBlocksLeavesPlainContentAlone(t *testing.T) {
	for _, content := range []string{
		`{"candidate": 1, "statement": "x", "coherence": 1.0}`,
		"Here is the answer: []",
		"",
		`[]`,
	} {
		if _, _, ok := flattenContentBlocks(content); ok {
			t.Errorf("%q must not be read as content blocks", content)
		}
	}
}

func TestChatReportsAReasoningOnlyAnswer(t *testing.T) {
	srv := blockServer(t, []any{thinkingBlock("Sources: Hermes, plesk, tailscale")}, "stop")
	r := newTriage(t, srv.URL)

	_, err := r.ReasonShapes(context.Background(), shapeCandidates()[:1])
	if err == nil {
		t.Fatal("want an error for an answer that is only a trace")
	}
	var reasoning *ReasoningOnlyError
	if !errors.As(err, &reasoning) {
		t.Fatalf("want a ReasoningOnlyError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "reasoning trace and no answer") {
		t.Errorf("the error must say what happened: %v", err)
	}
	if !IsAnswerError(err) {
		t.Error("a reasoning-only answer is an answer problem, not a call problem")
	}
}

// When the trace ran into the output limit, the budget is the cause and the
// error has to name it instead of blaming the answer.
func TestChatReportsTheBudgetWhenTheTraceRanOut(t *testing.T) {
	srv := blockServer(t, []any{thinkingBlock("Let me analyze")}, "length")
	r := newTriage(t, srv.URL)

	_, err := r.ReasonShapes(context.Background(), shapeCandidates()[:1])
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "output tokens without answering") {
		t.Fatalf("want the budget error, got %v", err)
	}
	if IsAnswerError(err) {
		t.Error("a budget that ran out is not an answer problem")
	}
}
