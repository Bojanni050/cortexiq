// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// valueKeyVersion prefixes every key. If the normalisation ever changes, old
// keys carry the old version and a backfill can tell which rows to recompute.
const valueKeyVersion = "v1:"

// valueKey returns the key under which a withdrawn claim is remembered, and
// false when the claim has no usable key.
//
// This is where the rejected-value tombstone is won or lost, and both ways of
// getting it wrong are quiet: too strict and it never fires, too loose and it
// blocks a legitimate update. So it is deliberately plain:
//
//   - NFKC first, so a fullwidth or otherwise "compatible" lookalike of a
//     letter compares as the letter. Stripping and lowercasing without it is
//     the bypass the Atlas red team found in round 9.
//   - Case folding rather than lowercasing, so "Straße" and "STRASSE" meet.
//   - Whitespace collapsed, and sentence-final punctuation trimmed. Only
//     that: "C++" must not become "C", and punctuation inside a claim stays.
//   - Diacritics stay. "café" and "cafe" are different words.
//
// A claim that normalises to nothing gets no key. Hashing the empty string
// would give every such claim the same key and let one of them block the rest.
//
// It does not catch a paraphrase. That is a known limit, not an oversight: a
// fuzzier key would also block a true restatement of a changed value.
func valueKey(claim string) (string, bool) {
	s := norm.NFKC.String(claim)
	s = cases.Fold().String(s)
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimRight(s, ".!?…;:, ")
	if s == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte(s))
	return valueKeyVersion + hex.EncodeToString(sum[:]), true
}
