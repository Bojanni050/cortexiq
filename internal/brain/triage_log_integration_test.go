package brain

// Integration tests for triage logging: with it on every triage call leaves one
// line with the verdict, never the episode text; with it off nothing is logged.
// Skipped unless CORTEXIQ_TEST_DSN is set, same as the rest of the integration
// suite.

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

// fakeTriage answers with a fixed verdict or error.
type fakeTriage struct {
	verdict *reasoner.TriageResult
	err     error
}

func (f fakeTriage) Triage(context.Context, []string) (*reasoner.TriageResult, error) {
	return f.verdict, f.err
}

const triageSecret = "the offsite storage box password rotation happens every quarter"

// captureLog redirects the standard logger for one test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(orig) })
	return &buf
}

func TestIntegrationTriageLogsItsVerdictWhenSwitchedOn(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.TriageLog = true })
	e.b.SetTriage(fakeTriage{verdict: &reasoner.TriageResult{Verdict: reasoner.TriageChatter, Reason: "only an acknowledgement"}})
	buf := captureLog(t)
	e.rememberEpisode(t, triageSecret)

	if _, err := e.b.Consolidate(e.ctx, e.ns); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"triage:", "namespace=" + e.ns, "episodes=1", "verdict=chatter", `reason="only an acknowledgement"`, "took="} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, triageSecret) || strings.Contains(out, "password rotation") {
		t.Errorf("the episode text leaked into the log:\n%s", out)
	}
}

func TestIntegrationTriageLogsAnErrorAndSaysItWasTreatedAsDurable(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.TriageLog = true })
	e.b.SetTriage(fakeTriage{err: errors.New("provider returned 503")})
	buf := captureLog(t)
	e.rememberEpisode(t, triageSecret)

	if _, err := e.b.Consolidate(e.ctx, e.ns); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "provider returned 503") || !strings.Contains(out, "treated as durable") {
		t.Errorf("log = %s; want the error and that it fails open", out)
	}
}

func TestIntegrationTriageLogsNothingWhenSwitchedOff(t *testing.T) {
	e := newEnv(t, nil)
	e.b.SetTriage(fakeTriage{verdict: &reasoner.TriageResult{Verdict: reasoner.TriageDurable, Reason: "a claim"}})
	buf := captureLog(t)
	e.rememberEpisode(t, triageSecret)

	if _, err := e.b.Consolidate(e.ctx, e.ns); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "triage:") {
		t.Errorf("triage logged although TriageLog is off:\n%s", buf.String())
	}
}
