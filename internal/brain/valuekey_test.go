package brain

import (
	"strings"
	"testing"
)

func mustKey(t *testing.T, claim string) string {
	t.Helper()
	k, ok := valueKey(claim)
	if !ok {
		t.Fatalf("valueKey(%q) has no key; want one", claim)
	}
	return k
}

func TestValueKeyIgnoresSurfaceVariation(t *testing.T) {
	base := mustKey(t, "the message queue is kafka")
	for _, variant := range []string{
		"The Message Queue Is Kafka",
		"  the   message\tqueue\nis kafka  ",
		"the message queue is kafka.",
		"the message queue is kafka!?",
		"the message queue is kafka …",
		// Fullwidth lookalikes: the round-9 bypass, a strip().lower() that
		// never saw NFKC.
		"ｔｈｅ ｍｅｓｓａｇｅ ｑｕｅｕｅ ｉｓ ｋａｆｋａ",
	} {
		if got := mustKey(t, variant); got != base {
			t.Errorf("valueKey(%q) differs from the plain claim; want the same key", variant)
		}
	}
}

func TestValueKeyKeepsDifferentClaimsApart(t *testing.T) {
	keys := map[string]string{}
	for _, claim := range []string{
		"the message queue is kafka",
		"the message queue is nats",
		"the language is C",
		"the language is C++", // trailing symbols are part of the claim
		"the cafe is open",
		"the café is open", // diacritics are part of the word
		"queue, kafka",     // punctuation inside a claim stays
		"queue kafka",
	} {
		k := mustKey(t, claim)
		if prev, dup := keys[k]; dup {
			t.Errorf("%q and %q share a key; want them apart", prev, claim)
		}
		keys[k] = claim
	}
}

func TestValueKeyFoldsCaseBeyondASCII(t *testing.T) {
	if mustKey(t, "Straße") != mustKey(t, "STRASSE") {
		t.Error("Straße and STRASSE should share a key under case folding")
	}
}

// The Atlas describes a system whose normaliser kept only ASCII letters and
// digits, so every claim in another script reduced to the empty string and
// hashed identically. Nothing attacked it; the tests never fed it a non-ASCII
// string. These do.
func TestValueKeyDoesNotCollapseOtherScripts(t *testing.T) {
	a := mustKey(t, "De wachtrij is kafka")
	b := mustKey(t, "消息队列是卡夫卡")
	c := mustKey(t, "очередь сообщений — это kafka")
	d := mustKey(t, "очередь сообщений — это nats")
	seen := map[string]bool{}
	for _, k := range []string{a, b, c, d} {
		if seen[k] {
			t.Fatal("two different claims share a key")
		}
		seen[k] = true
	}
}

func TestValueKeyHasNoKeyForEmptyClaims(t *testing.T) {
	for _, claim := range []string{"", "   ", "\n\t", "...", "?!", " … "} {
		if k, ok := valueKey(claim); ok {
			t.Errorf("valueKey(%q) = %q; want no key, or one empty claim would block every other", claim, k)
		}
	}
}

func TestValueKeyIsVersionedAndStable(t *testing.T) {
	k := mustKey(t, "the message queue is kafka")
	if !strings.HasPrefix(k, valueKeyVersion) {
		t.Errorf("key %q lacks the version prefix %q", k, valueKeyVersion)
	}
	if k != mustKey(t, "the message queue is kafka") {
		t.Error("the same claim gave two different keys")
	}
	if len(k) != len(valueKeyVersion)+64 {
		t.Errorf("key length = %d; want prefix plus 64 hex characters", len(k))
	}
}
