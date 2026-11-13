package brain

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// rerankCount reads recall_rerank_requests_total for one status.
func rerankCount(t *testing.T, status string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != "recall_rerank_requests_total" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "status" && l.GetValue() == status {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	t.Fatalf("recall_rerank_requests_total{status=%q} is not registered", status)
	return 0
}

// A reranker that fails must not fail the recall, but it must be visible: the
// fused order comes back unchanged and the error counter goes up.
func TestRerankFailureFallsBackAndIsCounted(t *testing.T) {
	rerankLastLog.Store(0)
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(orig) })

	before := rerankCount(t, "error")
	f := &fakeReranker{err: errors.New("reranker: 403 Forbidden: insufficient balance")}
	b := &Brain{config: Config{Reranker: f}}
	rs := []RecallResult{{ID: 1, Score: 0.9}, {ID: 2, Score: 0.8}}
	got := b.rerank(context.Background(), "q", rs)

	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 || got[0].Score != 0.9 {
		t.Fatalf("rerank after a failure = %+v, want the fused order and scores untouched", got)
	}
	if d := rerankCount(t, "error") - before; d != 1 {
		t.Fatalf("error counter moved by %v, want 1", d)
	}
	if !strings.Contains(buf.String(), "reranker failed") || !strings.Contains(buf.String(), "insufficient balance") {
		t.Fatalf("log = %q, want a line naming the failure", buf.String())
	}
}

func TestRerankSuccessIsCounted(t *testing.T) {
	before := rerankCount(t, "success")
	f := &fakeReranker{scores: map[int]float32{0: 0.1, 1: 0.9}}
	b := &Brain{config: Config{Reranker: f}}
	b.rerank(context.Background(), "q", []RecallResult{{ID: 1, Score: 0.9}, {ID: 2, Score: 0.8}})
	if d := rerankCount(t, "success") - before; d != 1 {
		t.Fatalf("success counter moved by %v, want 1", d)
	}
}

// A dead reranker fails on every recall: every failure is counted, one line is
// logged per interval.
func TestRerankFailureLogIsRateLimited(t *testing.T) {
	rerankLastLog.Store(0)
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(orig) })

	before := rerankCount(t, "error")
	b := &Brain{config: Config{Reranker: &fakeReranker{err: errors.New("down")}}}
	rs := func() []RecallResult { return []RecallResult{{ID: 1, Score: 0.9}, {ID: 2, Score: 0.8}} }
	for i := 0; i < 5; i++ {
		b.rerank(context.Background(), "q", rs())
	}
	if d := rerankCount(t, "error") - before; d != 5 {
		t.Fatalf("error counter moved by %v, want 5", d)
	}
	if n := strings.Count(buf.String(), "reranker failed"); n != 1 {
		t.Fatalf("logged %d lines for 5 failures, want 1:\n%s", n, buf.String())
	}
}
