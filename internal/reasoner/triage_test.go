// Modifications copyright (c) 2026 Bojan van der Heide
package reasoner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// triageServer answers every chat completion with the given content and records
// the request body, so a test can assert both the verdict and what was sent.
func triageServer(t *testing.T, content string) (*httptest.Server, *map[string]any) {
	t.Helper()
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s, want /chat/completions", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1,
			"model":   "cheap",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]string{"role": "assistant", "content": content},
				"finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &body
}

func newTriage(t *testing.T, url string) *OpenAI {
	t.Helper()
	r, err := NewOpenAI(url, "key", "cheap", 200, true)
	if err != nil {
		t.Fatalf("NewOpenAI: %v", err)
	}
	return r
}

func TestTriageReadsVerdict(t *testing.T) {
	srv, body := triageServer(t, `{"verdict":"chatter","reason":"only an acknowledgement"}`)
	r := newTriage(t, srv.URL)

	res, err := r.Triage(context.Background(), []string{"ok thanks!", "done"})
	if err != nil {
		t.Fatalf("Triage: %v", err)
	}
	if res.Verdict != TriageChatter {
		t.Errorf("verdict = %q, want chatter", res.Verdict)
	}
	if res.Reason != "only an acknowledgement" {
		t.Errorf("reason = %q", res.Reason)
	}
	if got := (*body)["model"]; got != "cheap" {
		t.Errorf("model = %v, want the triage model", got)
	}
	msgs, _ := (*body)["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want system + user", len(msgs))
	}
	user, _ := msgs[1].(map[string]any)
	if !strings.Contains(user["content"].(string), "done") {
		t.Errorf("user message does not carry the text: %v", user["content"])
	}
}

// A triage model that fails open is the whole safety story: a bad answer may
// cost money, never a memory.
func TestTriageFailsOpen(t *testing.T) {
	cases := map[string]string{
		"unparseable":     "I think this is chatter?",
		"unknown verdict": `{"verdict":"maybe","reason":"unclear"}`,
		"missing reason":  `not json at all {`,
		"empty":           ``,
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			srv, _ := triageServer(t, answer)
			r := newTriage(t, srv.URL)
			res, err := r.Triage(context.Background(), []string{"the deploy moved to 03:43"})
			if err != nil {
				t.Fatalf("Triage: %v", err)
			}
			if res.Verdict != TriageDurable {
				t.Errorf("verdict = %q, want durable on a failed answer", res.Verdict)
			}
		})
	}
}

func TestTriageDurablePassesThrough(t *testing.T) {
	srv, _ := triageServer(t, `{"verdict":"durable","reason":"a decision with a time"}`)
	r := newTriage(t, srv.URL)
	res, err := r.Triage(context.Background(), []string{"we moved the backup window to 03:43"})
	if err != nil {
		t.Fatalf("Triage: %v", err)
	}
	if res.Verdict != TriageDurable || res.Reason != "a decision with a time" {
		t.Errorf("unexpected result: %+v", res)
	}
}
