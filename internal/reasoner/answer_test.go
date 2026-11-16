// Modifications copyright (c) 2026 Bojan van der Heide
package reasoner

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/models"
)

const providerFailedGeneration = `{"error": "The previous response was invalid because it contained a placeholder. Please run the extraction again."}`

func TestProviderContentErrorReadsALoneErrorObject(t *testing.T) {
	got := providerContentError(`{"error": "Please run the extraction again."}`)
	if got == nil || got.Message != "Please run the extraction again." {
		t.Fatalf("got %+v, want the provider's message", got)
	}
}

func TestProviderContentErrorIgnoresAnswersWithRealData(t *testing.T) {
	for name, output := range map[string]string{
		"data next to an error": `{"error": "warning", "relationships": []}`,
		"non-string error":      `{"error": {"code": 500}}`,
		"empty error":           `{"error": "  "}`,
		"empty object":          `{}`,
		"an array":              `[{"fact_id": 1}]`,
		"prose":                 `The extraction produced {"error": "x"} and then prose.`,
	} {
		if got := providerContentError(output); got != nil {
			t.Errorf("%s: got %+v, want nil", name, got)
		}
	}
}

// chatCompletion wraps content in the shape the provider returns for a successful
// call: HTTP 200 and no finish_reason that says anything went wrong.
func chatCompletion(content string) string {
	return `{"id":"x","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":` + strconvQuote(content) + `}}],"usage":{"prompt_tokens":10,"completion_tokens":40,"total_tokens":50}}`
}

func strconvQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func TestChatReportsAProviderFailedGeneration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletion(providerFailedGeneration)))
	}))
	defer srv.Close()

	o, err := NewOpenAI(srv.URL, "k", "baseten/DeepSeek-V4-Flash-0731", 8192, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = o.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}})
	var failed *ProviderContentError
	if !errors.As(err, &failed) {
		t.Fatalf("want a ProviderContentError, got %v", err)
	}
	if !strings.Contains(err.Error(), "Please run the extraction again") {
		t.Errorf("the provider's own words must survive: %v", err)
	}
	if !IsAnswerError(err) {
		t.Error("a failed generation is an answer problem, not a transport problem")
	}
}

// A failed generation asks for the same extraction again. Appending the "your
// previous answer was not valid JSON" warning answers a complaint the provider
// never made, so the retry must resend the request unchanged.
func TestReasonRelationshipsBatchRetriesAFailedGenerationUnchanged(t *testing.T) {
	var calls int32
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		w.Header().Set("Content-Type", "application/json")
		if atomic.AddInt32(&calls, 1) == 1 {
			_, _ = w.Write([]byte(chatCompletion(providerFailedGeneration)))
			return
		}
		_, _ = w.Write([]byte(chatCompletion(`[{"fact_id": 1, "from": "Alice", "relation_type": "works_at", "to": "TechCorp", "confidence": 0.9}]`)))
	}))
	defer srv.Close()

	o, err := NewOpenAI(srv.URL, "k", "m", 8192, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := o.ReasonRelationshipsBatch(context.Background(), []models.Fact{{ID: 1, Content: "Alice works at TechCorp"}})
	if err != nil {
		t.Fatalf("the retry should have recovered, got %v", err)
	}
	if len(got[1]) != 1 || got[1][0].ToEntity != "TechCorp" {
		t.Fatalf("relationships = %+v, want Alice works_at TechCorp", got)
	}
	if len(bodies) != 2 {
		t.Fatalf("calls = %d, want 2", len(bodies))
	}
	if strings.Contains(bodies[1], retryWarning[:40]) {
		t.Error("the retry warned about an invalid answer; the provider failed on its own side")
	}
	if bodies[0] != bodies[1] {
		t.Error("the retry must resend the same request")
	}
}
