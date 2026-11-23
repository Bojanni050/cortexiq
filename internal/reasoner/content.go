// Modifications copyright (c) 2026 Bojan van der Heide
package reasoner

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Some providers return an assistant message as content blocks instead of a
// string: a reasoning block first, then the answer as a text block.
//
//	[{"type":"thinking","thinking":[{"type":"text","text":"Let me analyze..."}]},
//	 {"type":"text","text":"[{\"candidate\":1,...}]"}]
//
// The OpenAI SDK reads content as a string and only loosely reports the
// mismatch, so the array arrives here stringified and every stage that expects
// JSON sees a block list instead. Production lost the whole shape sweep to it
// on 2026-11-23: "parse json: unreadable answer to 1 shape candidates:
// [{\"type\":\"thinking\",...". The answer was there, one block further along.
type contentBlock struct {
	Type     string        `json:"type"`
	Text     string        `json:"text"`
	Thinking []contentText `json:"thinking"`
}

type contentText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// flattenContentBlocks turns a content-block answer into the text it carries.
// ok is false when the content is not a block list, so the caller keeps the
// answer exactly as it arrived.
//
// Only blocks named "text" and "thinking" are recognised as blocks. Anything
// else is left alone, because the failure stage answers with objects that have
// a "type" field of their own ([{"type":"repeated_failure",...}]) and reading
// those as blocks would erase a valid answer.
//
// Reasoning is dropped on purpose: a judge that picks JSON out of a model's
// thoughts is guessing at an answer the model never gave. When nothing but
// thinking came back, reasoningOnly says so and the caller can report it.
func flattenContentBlocks(content string) (text string, reasoningOnly, ok bool) {
	raw := strings.TrimSpace(content)
	if !strings.HasPrefix(raw, "[") {
		return "", false, false
	}

	blocks, decoded := decodeContentBlocks(raw)
	if !decoded {
		// A trace that ran into the output limit is cut off mid-block, so it
		// cannot be decoded; it is still a trace and not an answer. Decoding
		// what is left of it would not make it one.
		if thinkingBlockPrefix.MatchString(raw) {
			return "", true, true
		}
		return "", false, false
	}
	if len(blocks) == 0 {
		return "", false, false
	}
	for _, b := range blocks {
		if b.Type != "text" && b.Type != "thinking" {
			return "", false, false
		}
	}

	var parts []string
	sawThinking := false
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if t := strings.TrimSpace(b.Text); t != "" {
				parts = append(parts, t)
			}
		case "thinking":
			sawThinking = true
		}
	}

	text = strings.Join(parts, "")
	return text, text == "" && sawThinking, true
}

// thinkingBlockPrefix recognises the start of a block list that opens with a
// reasoning block, which is what a trace cut off by the output limit looks like.
var thinkingBlockPrefix = regexp.MustCompile(`^\[\s*\{\s*"type"\s*:\s*"thinking"\s*,`)

// decodeContentBlocks reads the block list, keeping the complete blocks of an
// answer that was cut off mid-block: a trace that ran into the token limit is
// still a trace, and reporting it as such is more useful than a parse error
// about a JSON array the model never meant as an answer.
func decodeContentBlocks(raw string) ([]contentBlock, bool) {
	var blocks []contentBlock
	if err := json.Unmarshal([]byte(raw), &blocks); err == nil {
		return blocks, true
	}
	if salvaged := repairTruncatedArray(raw); salvaged != raw {
		if err := json.Unmarshal([]byte(salvaged), &blocks); err == nil {
			return blocks, true
		}
	}
	return nil, false
}
