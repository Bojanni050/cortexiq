// Modifications copyright (c) 2026 Bojan van der Heide
package reasoner

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// decodeModelJSON unmarshals model output, first repairing a glitch models do
// produce: whitespace inside a numeric literal ("0. 9", "1 .5", "- 3"), which
// encoding/json rejects with "invalid character ' ' after decimal point in
// numeric literal". Without the repair one stray space costs a whole batch of
// facts its relationships, and the next run asks the model again and may get the
// same answer.
//
// The repair is deliberately narrow: only whitespace that sits inside a number,
// outside any string, is removed. Nothing is guessed or invented, and when it
// still fails the error quotes the neighbourhood of the syntax error so a bad
// answer can be diagnosed without logging a whole memory.
func decodeModelJSON(raw string, v any) error {
	err := json.Unmarshal([]byte(raw), v)
	if err == nil {
		return nil
	}
	if fixed := repairNumberSpacing(raw); fixed != raw {
		if retryErr := json.Unmarshal([]byte(fixed), v); retryErr == nil {
			return nil
		}
	}
	// A model that runs out of output tokens stops mid-object. Throwing away the
	// whole batch because the last element is incomplete loses knowledge that is
	// already written down, so keep the complete prefix of the array.
	if salvaged := repairTruncatedArray(raw); salvaged != raw {
		if retryErr := json.Unmarshal([]byte(salvaged), v); retryErr == nil {
			return nil
		}
	}
	return fmt.Errorf("%w (near %q)", err, nearJSONError(raw, err))
}

// repairNumberSpacing removes whitespace inside a numeric literal. A gap between
// a digit and a '.', between a '.' and a digit, or after a leading '-' can never
// mean anything else in JSON, so dropping it is safe. Whitespace elsewhere is
// preserved verbatim, and the input is returned unchanged when there is nothing
// to fix — that keeps a well-formed answer byte-identical.
func repairNumberSpacing(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	inString, escaped, changed := false, false, false
	var last byte // last byte written outside a string, for the adjacency test

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			out.WriteByte(c)
			switch {
			case escaped:
				escaped = false
			case c == 92: // backslash
				escaped = true
			case c == '"':
				inString = false
				last = c
			}
			continue
		}
		if c == '"' {
			inString = true
			out.WriteByte(c)
			last = c
			continue
		}
		if isJSONSpace(c) {
			j := i
			for j < len(s) && isJSONSpace(s[j]) {
				j++
			}
			var next byte
			if j < len(s) {
				next = s[j]
			}
			if insideNumber(last, next) {
				changed = true
				i = j - 1
				continue
			}
			for k := i; k < j; k++ {
				out.WriteByte(s[k])
			}
			last = s[j-1]
			i = j - 1
			continue
		}
		out.WriteByte(c)
		last = c
	}
	if !changed {
		return s
	}
	return out.String()
}

func isJSONSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// insideNumber reports whether whitespace between prev and next is inside a
// numeric literal.
func insideNumber(prev, next byte) bool {
	return isDigit(prev) && next == '.' ||
		prev == '.' && isDigit(next) ||
		prev == '-' && isDigit(next)
}

// nearJSONError quotes the bytes around a syntax error, so the log says where the
// model went wrong instead of only that it did.
func nearJSONError(raw string, err error) string {
	var se *json.SyntaxError
	if errors.As(err, &se) {
		lo := int(se.Offset) - 40
		if lo < 0 {
			lo = 0
		}
		hi := int(se.Offset) + 40
		if hi > len(raw) {
			hi = len(raw)
		}
		if lo < hi {
			return raw[lo:hi]
		}
	}
	if len(raw) > 120 {
		return raw[:120]
	}
	return raw
}

// repairTruncatedArray closes an array that was cut off mid-element, keeping the
// elements that were fully written. Only the last, incomplete element is dropped:
// everything before it is valid JSON the model really produced.
//
// It returns the input unchanged when the array is not obviously truncated, so a
// complete answer is never rewritten.
func repairTruncatedArray(s string) string {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "[") || strings.HasSuffix(trimmed, "]") {
		return s
	}
	depth, inString, escaped := 0, false, false
	lastComplete := -1
	for i := 0; i < len(trimmed); i++ {
		c := trimmed[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == 92: // backslash
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			// depth 1 means the element closed but the outer array is still open.
			if depth == 1 && c == '}' {
				lastComplete = i + 1
			}
		}
	}
	if lastComplete <= 1 {
		return s
	}
	return trimmed[:lastComplete] + "]"
}

// firstJSONValue returns the first complete JSON object or array in s, and s
// itself when it finds none. A model that closes its answer and then keeps
// talking used to fail with "invalid character after top-level value": the
// extraction returned the whole reply, closing fence and commentary included.
func firstJSONValue(s string) string {
	start := -1
	for i := 0; i < len(s); i++ {
		if s[i] == '[' || s[i] == '{' {
			start = i
			break
		}
	}
	if start < 0 {
		return s
	}

	depth, inString, escaped := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == 92: // backslash
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return s
}
