// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"errors"
	"testing"
)

// The list is deliberately narrow: a closer that slips through costs one episode,
// a real memory that is swallowed is lost. Every entry on the "keep" side is a
// short message that must survive.
func TestSocialCloserRecognisesPhaticContent(t *testing.T) {
	keep := []string{
		"ja doe maar",
		"push naar main",
		"nee, dat werkt niet",
		"het is niet gelukt",
		"CortexIQ draait op poort 9090",
		"Deploy staat live op Fly.io",
		"Waarom werkt de consolidator niet?",
		"dank je wel voor de hulp met de migratie",
		"morgen om 9 uur call",
		"ok, dan refactor je de recall-pijplijn",
		"top, ik merge hem zelf",
		"prima, ga zo door",
		"Do you remember what we decided?",
		"dank! kunnen we ook dedupe op entity doen",
	}
	for _, content := range keep {
		if socialCloser(content) {
			t.Errorf("socialCloser(%q) = true; this carries content and must be stored", content)
		}
	}

	skip := []string{
		"",
		"...",
		"👍",
		"👍👍 🙏",
		"ok",
		"oké",
		"nee",
		"dank je wel!",
		"Thanks, much appreciated",
		"thanks!",
		"prima",
		"top, bedankt",
		"prima, dank je wel, tot morgen",
		"oké, duidelijk",
		"hoi",
		"helemaal eens",
		"dat is goed",
		"tot ziens en bedankt voor alles",
		"great, thanks a lot",
	}
	for _, content := range skip {
		if !socialCloser(content) {
			t.Errorf("socialCloser(%q) = false; this says nothing and must be refused", content)
		}
	}
}

// A long message with a subject survives even when it opens politely.
func TestSocialCloserStopsAtContentLength(t *testing.T) {
	long := "dank je wel voor het uitzoeken van de migratie naar pgvector en de dedupe-ronde"
	if socialCloser(long) {
		t.Error("a long message with a subject must not be treated as phatic")
	}
}

// The refusal must happen before the embedder: no embedding call is the whole
// point of the gate. The per-namespace lookup runs against the pool, so this
// test needs a database; the pure gate is covered by socialCloser's own tests.
func TestRememberRefusesSocialCloser(t *testing.T) {
	e := newEnv(t, nil)
	if _, err := e.b.Remember(e.ctx, e.ns, "dank je wel!", nil); !errors.Is(err, ErrSocialCloser) {
		t.Errorf("err = %v, want ErrSocialCloser", err)
	}
}

// With the filter off the same write is allowed through; that path needs a real
// database, so it lives in social_integration_test.go.
