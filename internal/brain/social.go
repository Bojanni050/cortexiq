// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"strings"
	"unicode"
)

// maxSocialWords bounds a message that may still be pure phatic content. Real
// sign-offs are short; a longer message has a subject, even when it opens with
// "thanks".
const maxSocialWords = 10

// socialWords is the closed vocabulary of a message that says nothing: greetings,
// thanks, acknowledgements and sign-offs, in the two languages this memory is fed
// in (Dutch and English), plus the function words that glue them together.
//
// Terseness is deliberately NOT the criterion — MinEpisodeLength already keeps
// short episodes from the extractor. This list only catches noise that is long
// enough to look like content: "prima, dank je wel, tot morgen". A decision or an
// instruction never matches, however short: "ja doe maar" survives because "doe"
// is a verb, "push maar" because "push" is work, "nee dat werkt niet" because
// "werkt" says something.
//
// Every word that is added here also widens what may be dropped, so the list
// stays tight on purpose: a closer that slips through costs one episode, a real
// memory that is swallowed is lost.
var socialWords = map[string]struct{}{
	// bare answers and acknowledgements
	"ok": {}, "oke": {}, "okay": {}, "oké": {}, "k": {}, "kk": {},
	"ja": {}, "nee": {}, "yes": {}, "yep": {}, "yeah": {}, "yup": {}, "no": {}, "nope": {},
	"duidelijk": {}, "helder": {}, "snap": {}, "snapte": {}, "begrepen": {}, "logisch": {},
	"klopt": {}, "klop": {}, "precies": {}, "inderdaad": {}, "eens": {}, "helemaal": {},
	"akkoord": {}, "deal": {}, "sure": {}, "right": {}, "exactly": {}, "indeed": {},
	"prima": {}, "goed": {}, "goedzo": {}, "fijn": {}, "mooi": {}, "top": {}, "topper": {},
	"perfect": {}, "great": {}, "good": {}, "nice": {}, "cool": {}, "super": {}, "awesome": {},
	"geweldig": {}, "lekker": {}, "held": {},
	"clear": {}, "got": {}, "done": {}, "works": {}, "working": {}, "sorted": {},
	// thanks and politeness
	"thanks": {}, "thank": {}, "thankyou": {}, "thx": {}, "ty": {}, "tx": {}, "cheers": {},
	"dank": {}, "dankje": {}, "bedankt": {}, "dankuwel": {}, "dankjewel": {},
	"please": {}, "asjeblieft": {}, "alsjeblieft": {}, "aub": {}, "graag": {},
	"appreciate": {}, "appreciated": {}, "waardeer": {},
	"geen": {}, "probleem": {}, "problem": {}, "worries": {}, "np": {},
	"hulp": {}, "voor": {}, "hoor": {}, "hè": {}, "he": {}, "even": {}, "gewoon": {},
	// greetings and sign-offs
	"hi": {}, "hello": {}, "hallo": {}, "hoi": {}, "hey": {}, "morgen": {}, "dag": {},
	"middag": {}, "avond": {}, "nacht": {}, "welterusten": {}, "fijne": {}, "sleep": {},
	"well": {}, "bye": {}, "doei": {}, "doeg": {}, "tot": {}, "ziens": {}, "later": {},
	"groet": {}, "groeten": {}, "groetjes": {}, "see": {}, "you": {}, "soon": {},
	// function words that carry no claim
	"de": {}, "het": {}, "een": {}, "is": {}, "was": {}, "dat": {}, "die": {}, "dit": {},
	"deze": {}, "ik": {}, "je": {}, "jij": {}, "u": {}, "we": {}, "wij": {}, "ze": {}, "zij": {},
	"me": {}, "mijn": {}, "men": {}, "er": {}, "hier": {}, "daar": {}, "nu": {}, "nog": {},
	"wel": {}, "niet": {}, "maar": {}, "ook": {}, "zo": {}, "al": {}, "dus": {}, "toch": {},
	"vast": {}, "veel": {}, "meer": {}, "keer": {}, "en": {}, "of": {}, "met": {}, "als": {},
	"it": {}, "that": {}, "this": {}, "so": {}, "much": {}, "very": {}, "all": {}, "and": {},
	"for": {}, "to": {}, "the": {}, "a": {}, "an": {}, "lot": {}, "alles": {},
}

// socialCloser reports whether content carries nothing worth remembering: it
// consists only of greetings, thanks, acknowledgements and the function words
// around them, and it asks nothing.
//
// A write that hits this is not stored at all: there is nothing to consolidate,
// recall would only surface it as noise, and an embedding call is the one cost a
// memory server should never pay for "dank je wel".
func socialCloser(content string) bool {
	// A question, a path, code or a handle means something was asked or named;
	// the vocabulary below can never describe that.
	if strings.ContainsAny(content, "?=@#`/\\<>{}[]") {
		return false
	}
	tokens := phaticTokens(content)
	if len(tokens) == 0 {
		return true // punctuation or emoji only
	}
	if len(tokens) > maxSocialWords {
		return false
	}
	for _, tok := range tokens {
		if _, ok := socialWords[tok]; !ok {
			return false
		}
	}
	return true
}

// phaticTokens lowercases content and splits it on everything that is not a
// letter or a digit, so punctuation and emoji act as separators instead of
// joining "dank," to "dank".
func phaticTokens(content string) []string {
	var (
		out []string
		cur strings.Builder
	)
	for _, r := range strings.ToLower(content) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
			continue
		}
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
