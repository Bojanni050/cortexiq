package bootstrap

import (
	"testing"

	"github.com/Bojanni050/cortexiq/internal/llmreg"
)

func TestBindingsFingerprintTracksWhatApplyingActsOn(t *testing.T) {
	providers := map[string]llmreg.Provider{
		"hopscotch": {Slug: "hopscotch", BaseURL: "https://hop/v1", APIKey: "k1"},
		"mistral":   {Slug: "mistral", BaseURL: "https://mistral/v1", APIKey: "k2"},
	}
	base := map[string]llmreg.Binding{
		llmreg.PurposeReasoner: {Purpose: llmreg.PurposeReasoner, Provider: "hopscotch", Model: "m1"},
		llmreg.PurposeTriage:   {Purpose: llmreg.PurposeTriage, Provider: "hopscotch", Model: "t1"},
	}
	before := bindingsFingerprint(base, providers)

	if got := bindingsFingerprint(base, providers); got != before {
		t.Fatal("the same input must give the same fingerprint")
	}

	// A timestamp is not a change.
	same := map[string]llmreg.Binding{
		llmreg.PurposeReasoner: {Purpose: llmreg.PurposeReasoner, Provider: "hopscotch", Model: "m1", Updated: "later"},
		llmreg.PurposeTriage:   base[llmreg.PurposeTriage],
	}
	if got := bindingsFingerprint(same, providers); got != before {
		t.Fatal("re-saving an unchanged binding must not look like a change")
	}

	// Embedding and reranker are not applied under a running process, so they
	// must not trigger a reload either.
	withEmbedding := map[string]llmreg.Binding{
		llmreg.PurposeReasoner:  base[llmreg.PurposeReasoner],
		llmreg.PurposeTriage:    base[llmreg.PurposeTriage],
		llmreg.PurposeEmbedding: {Purpose: llmreg.PurposeEmbedding, Provider: "mistral", Model: "e"},
	}
	if got := bindingsFingerprint(withEmbedding, providers); got != before {
		t.Fatal("an embedding binding must not change the fingerprint")
	}

	changes := map[string]func(){
		"another provider": func() {
			b := base[llmreg.PurposeReasoner]
			b.Provider = "mistral"
			base[llmreg.PurposeReasoner] = b
		},
		"another model": func() {
			b := base[llmreg.PurposeReasoner]
			b.Model = "m2"
			base[llmreg.PurposeReasoner] = b
		},
		"a rotated key": func() {
			p := providers["hopscotch"]
			p.APIKey = "k9"
			providers["hopscotch"] = p
		},
		"another base url": func() {
			p := providers["hopscotch"]
			p.BaseURL = "https://elsewhere/v1"
			providers["hopscotch"] = p
		},
	}
	for name, change := range changes {
		// Start each case from the same state.
		base[llmreg.PurposeReasoner] = llmreg.Binding{Purpose: llmreg.PurposeReasoner, Provider: "hopscotch", Model: "m1"}
		providers["hopscotch"] = llmreg.Provider{Slug: "hopscotch", BaseURL: "https://hop/v1", APIKey: "k1"}
		change()
		if got := bindingsFingerprint(base, providers); got == before {
			t.Errorf("%s must change the fingerprint", name)
		}
	}
}
