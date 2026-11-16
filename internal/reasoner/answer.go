// Modifications copyright (c) 2026 Bojan van der Heide
package reasoner

import (
	"encoding/json"
	"errors"
	"strings"
)

// AnswerError marks a failure that is about the model's answer rather than about
// reaching the model: the reply arrived, it just cannot be read. The distinction
// matters to a caller that has to decide between asking again and moving on — a
// provider that is down usually recovers, an answer shape that keeps arriving the
// same way does not.
type AnswerError struct {
	Msg string
	Err error
}

func (e *AnswerError) Error() string { return e.Msg + ": " + e.Err.Error() }
func (e *AnswerError) Unwrap() error { return e.Err }

// parseError keeps the message of a parse failure exactly what it was before this
// type existed, so logs and existing tests read the same while errors.As can see
// what kind of failure it is.
func parseError(err error) error { return &AnswerError{Msg: "parse json", Err: err} }

// ProviderContentError is a generation the provider itself marked as failed and
// returned as the message content:
//
//	{"error": "The previous response was invalid because it contained a placeholder. Please run the extraction again."}
//
// It arrives with HTTP 200, no transport error and no finish_reason that says
// anything is wrong, so before this it surfaced as "parse json: cannot unmarshal
// object into Go value of type []reasoner.jsonRelationship" at whichever stage
// made the call. The provider asks for the extraction to be run again, so
// resending the same request is the useful reaction; a stage that appends a
// "your previous answer was not valid JSON" warning is answering a complaint the
// provider never made.
type ProviderContentError struct{ Message string }

func (e *ProviderContentError) Error() string {
	return "reasoner: the provider reported a failed generation: " + e.Message
}

// providerContentError reads content that is a lone {"error": "..."} object. Only
// that exact shape counts: a document with an "error" field next to real data is
// not a failed generation, and prose that merely mentions an error is not either.
func providerContentError(output string) *ProviderContentError {
	raw := strings.TrimSpace(extractJSON(output))
	if !strings.HasPrefix(raw, "{") {
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil || len(obj) != 1 {
		return nil
	}
	rawMessage, ok := obj["error"]
	if !ok {
		return nil
	}
	var message string
	if err := json.Unmarshal(rawMessage, &message); err != nil {
		return nil
	}
	if message = strings.TrimSpace(message); message == "" {
		return nil
	}
	return &ProviderContentError{Message: message}
}

// IsAnswerError reports whether err is about the shape of the answer instead of
// about the call. A transport failure, a timeout and a budget that ran out are
// not answer errors: the same batch may well be extracted on the next run.
func IsAnswerError(err error) bool {
	var answer *AnswerError
	var content *ProviderContentError
	return errors.As(err, &answer) || errors.As(err, &content)
}
