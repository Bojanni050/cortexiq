package main

import (
	"context"
	"strings"

	"github.com/Bojanni050/cortexiq/internal/brain"
)

// untaggedHint is what a writer reads back when it stored a memory without
// tags. The write succeeded, so this is not an error: it is the one moment the
// writer is still looking, and the vocabulary of that namespace is what it
// needs to pick tags instead of inventing a word that fragments the filter.
//
// The lookup is best effort. Failing the call because the vocabulary could not
// be read would throw away a memory that was written successfully.
func untaggedHint(ctx context.Context, b *brain.Brain, namespace string) string {
	vocab, err := b.ListTags(ctx, []string{namespace}, nil, 8)
	if err != nil {
		vocab = nil
	}
	return untaggedHintText(tagNames(vocab))
}

// tagNames reduces the vocabulary to its tags, in the order ListTags returned
// them (most used first).
func tagNames(counts []brain.TagCount) []string {
	names := make([]string, 0, len(counts))
	for _, c := range counts {
		names = append(names, c.Tag)
	}
	return names
}

// untaggedHintText is untaggedHint without the database, so the wording can be
// tested on its own.
func untaggedHintText(known []string) string {
	const lead = "Stored without tags. Tags are what keeps this memory findable once consolidation has replaced the raw episode with facts, and a tag only filters if it is spelled the way the rest of the store spells it. Pass them comma-separated in tags, for example tags=\"beslissing,bugfix\"."
	if len(known) == 0 {
		return lead + " This namespace has no tags yet; call list_tags before the next write to see what appears."
	}
	return lead + " This namespace already uses: " + strings.Join(known, ", ") + "."
}
