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

// unwrapTopLevelArray accepts the two shapes a model returns for a call that
// asks for a JSON array: the bare array, and an object that wraps it
// ({"patterns": [...]}). A model told to answer with "only valid JSON" often
// reaches for a complete document instead of a bare value.
//
// Production hit this on the patterns call: every run failed with "cannot
// unmarshal object into Go value of type []reasoner.jsonPattern", so no pattern
// was stored and the checkpoint never advanced. The array under the preferred
// key wins; otherwise the first array directly inside the object is taken.
//
// The scan is textual on purpose. Decoding the object to look inside it would
// fail on exactly the answers that need repair ("0. 9"), which is when a
// wrapped array is most likely to arrive; the caller still runs the result
// through decodeModelJSON, so the repairs keep working. Input that is already
// an array, or that wraps no array at all, is returned unchanged so a real
// parse error still surfaces.
func unwrapTopLevelArray(raw, key string) string {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") {
		return raw
	}
	if arr, ok := arrayAfterKey(trimmed, key); ok {
		return arr
	}
	if arr, ok := firstArrayInObject(trimmed); ok {
		return arr
	}
	// One candidate sometimes arrives as the element itself rather than as the
	// array that holds it: {"pattern": ...}. Wrapping it keeps the answer
	// usable instead of losing it to a shape difference.
	if strings.Contains(trimmed, `"pattern"`) {
		return "[" + trimmed + "]"
	}
	return raw
}

// arrayAfterKey returns the array that follows "key" in an object, or false when
// the key is absent or its value is not an array.
func arrayAfterKey(s, key string) (string, bool) {
	idx := strings.Index(s, `"`+key+`"`)
	if idx < 0 {
		return "", false
	}
	open := strings.IndexByte(s[idx:], '[')
	if open < 0 {
		return "", false
	}
	open += idx
	// An array can only be the value of this key when nothing but a colon sits
	// between the key and it; a '[1]' deeper inside another value is not ours.
	if between := strings.TrimSpace(s[idx+len(key)+2 : open]); between != ":" {
		return "", false
	}
	return balancedArray(s, open), true
}

// firstArrayInObject returns the first array of objects that is a direct member
// of the object s, or false when it holds none. An array of scalars is skipped
// on purpose: a model that answers with an object also tends to list the ids it
// used ([2605, 2606, 2607]), and taking that array as the answer produced
// "cannot unmarshal number into Go value of type reasoner.jsonPattern".
func firstArrayInObject(s string) (string, bool) {
	depth, inString, escaped := 0, false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == 92:
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
		case '[':
			if depth == 1 && arrayHoldsObjects(s, i) {
				return balancedArray(s, i), true
			}
		}
	}
	return "", false
}

// arrayHoldsObjects reports whether the array opened at open is empty or starts
// with an object.
func arrayHoldsObjects(s string, open int) bool {
	for i := open + 1; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
			continue
		case '{', ']':
			return true
		default:
			return false
		}
	}
	return true
}

// balancedArray returns s[open:] up to and including the bracket that closes the
// array opened at open, and s[open:] when the answer was cut off. Handing back
// the unterminated tail lets repairTruncatedArray salvage the complete elements.
func balancedArray(s string, open int) string {
	depth, inString, escaped := 0, false, false
	for i := open; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == 92:
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
				if c == ']' {
					return s[open : i+1]
				}
				return s[open:i]
			}
		}
	}
	return s[open:]
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
