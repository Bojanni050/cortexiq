package main

import (
	"strings"
	"testing"
)

// The hint is the only thing a writer sees when it forgot tags, so the wording
// carries the enforcement: it must say that the memory was stored, and it must
// hand over a vocabulary to reuse rather than leave the writer to invent one.
func TestUntaggedHintText(t *testing.T) {
	withoutVocabulary := untaggedHintText(nil)
	if !strings.Contains(withoutVocabulary, "Stored without tags") {
		t.Errorf("hint should say the memory was stored: %q", withoutVocabulary)
	}
	if !strings.Contains(withoutVocabulary, "list_tags") {
		t.Errorf("hint without a vocabulary should point at list_tags: %q", withoutVocabulary)
	}
	if strings.Contains(withoutVocabulary, "already uses") {
		t.Errorf("hint without a vocabulary claims one: %q", withoutVocabulary)
	}

	withVocabulary := untaggedHintText([]string{"beslissing", "bugfix"})
	if !strings.Contains(withVocabulary, "beslissing, bugfix") {
		t.Errorf("hint should name the vocabulary in order: %q", withVocabulary)
	}
	if !strings.Contains(withVocabulary, `tags="`) {
		t.Errorf("hint should show how to pass tags: %q", withVocabulary)
	}
	if !strings.HasSuffix(withVocabulary, ".") {
		t.Errorf("hint should end in a sentence: %q", withVocabulary)
	}
	if strings.Contains(withVocabulary, "uses: .") {
		t.Errorf("hint left an empty list behind: %q", withVocabulary)
	}
}

func TestTagNames(t *testing.T) {
	if got := tagNames(nil); len(got) != 0 {
		t.Errorf("tagNames(nil) = %v, want none", got)
	}
}
