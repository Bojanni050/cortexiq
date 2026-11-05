package brain

// Integration tests for the rejected-value tombstone in insertFact. Skipped
// unless CORTEXIQ_TEST_DSN is set, same as the rest of the integration suite.
// TestIntegrationRejectedValueStaysRejected covers the end-to-end sequence
// through consolidation; these write straight through insertFact, the way every
// path does, so a claim reaches the check exactly as written.

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func (e *testEnv) tryInsertFact(t *testing.T, nsID int64, content string) error {
	t.Helper()
	vec, _ := fakeEmbedder{}.Embed(e.ctx, content)
	// originImport: checked by the tombstone like every other path, and asks for
	// no source, which a bare test fact does not have.
	_, err := e.b.insertFact(e.ctx, e.b.pool, factInsert{
		Origin:         originImport,
		NamespaceID:    nsID,
		Content:        content,
		Embedding:      vec,
		EmbeddingModel: "fake",
		Confidence:     0.8,
		Importance:     DefaultImportance,
	})
	return err
}

func (e *testEnv) rejectClaim(t *testing.T, claim string) *Retraction {
	t.Helper()
	r, err := e.b.RetractFact(e.ctx, e.insertFact(t, claim, nil), "it is nats since March", "test")
	if err != nil {
		t.Fatalf("RetractFact(%q): %v", claim, err)
	}
	return r
}

const kafkaClaim = "the message queue between the services is kafka"

func TestIntegrationTombstoneBlocksSurfaceVariants(t *testing.T) {
	e := newEnv(t, nil)
	e.rejectClaim(t, kafkaClaim)

	for _, variant := range []string{
		kafkaClaim,
		"The Message Queue Between The Services Is Kafka.",
		"  the   message queue between\tthe services is kafka  ",
		"ｔｈｅ ｍｅｓｓａｇｅ ｑｕｅｕｅ ｂｅｔｗｅｅｎ ｔｈｅ ｓｅｒｖｉｃｅｓ ｉｓ ｋａｆｋａ",
	} {
		if err := e.tryInsertFact(t, e.nsID, variant); !errors.Is(err, ErrRejectedValue) {
			t.Errorf("insert of %q err = %v; want ErrRejectedValue", variant, err)
		}
	}
}

func TestIntegrationTombstoneErrorSaysWhy(t *testing.T) {
	e := newEnv(t, nil)
	r := e.rejectClaim(t, kafkaClaim)

	err := e.tryInsertFact(t, e.nsID, kafkaClaim)
	var rv *RejectedValueError
	if !errors.As(err, &rv) {
		t.Fatalf("err = %v; want a *RejectedValueError", err)
	}
	if rv.RetractionID != r.ID || !strings.Contains(rv.Reason, "nats") {
		t.Fatalf("error = %+v; want the retraction %d and its reason", rv, r.ID)
	}
}

// A different claim, a similar claim and a claim that merely shares words must
// all get in: the tombstone is on a value, not on a topic.
func TestIntegrationTombstoneLeavesOtherClaimsAlone(t *testing.T) {
	e := newEnv(t, nil)
	e.rejectClaim(t, kafkaClaim)

	for _, other := range []string{
		"the message queue between the services is nats",
		"the message queue between the services is kafka again",
		"the cache in front of the database is redis",
	} {
		if err := e.tryInsertFact(t, e.nsID, other); err != nil {
			t.Errorf("insert of %q err = %v; want it stored", other, err)
		}
	}
}

func TestIntegrationTombstoneStaysInItsNamespace(t *testing.T) {
	e := newEnv(t, nil)
	e.rejectClaim(t, kafkaClaim)

	other := e.ns + "-other"
	otherID, err := e.b.CreateNamespace(e.ctx, other, other, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.tryInsertFact(t, otherID, kafkaClaim); err != nil {
		t.Fatalf("insert in another namespace err = %v; a rejection in one project must not follow into another", err)
	}
}

// A normaliser that reduced every non-Latin claim to the empty string would let
// one rejection block them all.
func TestIntegrationTombstoneOnOneScriptDoesNotBlockAnother(t *testing.T) {
	e := newEnv(t, nil)
	e.rejectClaim(t, "消息队列是卡夫卡")

	if err := e.tryInsertFact(t, e.nsID, "消息队列是卡夫卡"); !errors.Is(err, ErrRejectedValue) {
		t.Errorf("the rejected non-Latin claim err = %v; want ErrRejectedValue", err)
	}
	for _, other := range []string{"备份每晚复制到异地存储", "очередь сообщений — это nats"} {
		if err := e.tryInsertFact(t, e.nsID, other); err != nil {
			t.Errorf("insert of %q err = %v; want it stored", other, err)
		}
	}
}

func TestIntegrationLiftedTombstoneLetsTheValueIn(t *testing.T) {
	e := newEnv(t, nil)
	r := e.rejectClaim(t, kafkaClaim)
	if err := e.tryInsertFact(t, e.nsID, kafkaClaim); !errors.Is(err, ErrRejectedValue) {
		t.Fatalf("precondition: err = %v; want ErrRejectedValue before the lift", err)
	}

	lifted, err := e.b.LiftRetraction(e.ctx, r.ID, "we moved back to kafka in October", "test")
	if err != nil {
		t.Fatalf("LiftRetraction: %v", err)
	}
	if lifted.LiftedAt == nil || lifted.LiftedBy == nil || *lifted.LiftedBy != "test" || lifted.LiftReason == nil {
		t.Fatalf("lifted = %+v; want who, when and why recorded", lifted)
	}
	if err := e.tryInsertFact(t, e.nsID, kafkaClaim); err != nil {
		t.Fatalf("insert after the lift err = %v; want it stored", err)
	}

	if _, err := e.b.LiftRetraction(e.ctx, r.ID, "again", "test"); !errors.Is(err, ErrAlreadyLifted) {
		t.Errorf("second lift err = %v; want ErrAlreadyLifted", err)
	}
	if _, err := e.b.LiftRetraction(e.ctx, r.ID+1_000_000, "x", "test"); !errors.Is(err, ErrRetractionNotFound) {
		t.Errorf("lift of an unknown id err = %v; want ErrRetractionNotFound", err)
	}
}

func TestIntegrationLiftNeedsAReason(t *testing.T) {
	e := newEnv(t, nil)
	r := e.rejectClaim(t, kafkaClaim)

	if _, err := e.b.LiftRetraction(e.ctx, r.ID, "   ", "test"); !errors.Is(err, ErrLiftReasonRequired) {
		t.Fatalf("lift without a reason err = %v; want ErrLiftReasonRequired", err)
	}
	// The refused lift changed nothing: the value is still blocked.
	if err := e.tryInsertFact(t, e.nsID, kafkaClaim); !errors.Is(err, ErrRejectedValue) {
		t.Fatalf("after a refused lift err = %v; want the tombstone to stand", err)
	}
}

func TestIntegrationEvalOriginIgnoresTheTombstone(t *testing.T) {
	e := newEnv(t, nil)
	e.rejectClaim(t, kafkaClaim)

	vec, _ := fakeEmbedder{}.Embed(e.ctx, kafkaClaim)
	if _, err := e.b.insertFact(e.ctx, e.b.pool, factInsert{
		Origin: originEval, NamespaceID: e.nsID, Content: kafkaClaim, Embedding: vec,
		EmbeddingModel: "fake", Confidence: 0.8, Importance: DefaultImportance,
	}); err != nil {
		t.Fatalf("eval-origin insert err = %v; the harness must not be blocked", err)
	}
}

// Keyed invalidate with a reason records a retraction, so it tombstones the
// claim; without a reason it records none, and the value may come back.
func TestIntegrationInvalidateTombstonesOnlyWithAReason(t *testing.T) {
	e := newEnv(t, nil)
	const without = "nightly backups are copied to the offsite storage box"

	for i, claim := range []string{kafkaClaim, without} {
		key := fmt.Sprintf("mirror:claim:%d", i)
		if _, err := e.b.RememberWithOutcome(e.ctx, e.ns, claim, RememberOptions{ExternalKey: key}); err != nil {
			t.Fatalf("RememberWithOutcome(%q): %v", claim, err)
		}
	}
	if _, err := e.b.InvalidateKeyed(e.ctx, e.ns, "mirror:claim:0", "it is nats since March", "test"); err != nil {
		t.Fatalf("InvalidateKeyed with reason: %v", err)
	}
	if _, err := e.b.InvalidateKeyed(e.ctx, e.ns, "mirror:claim:1", "", "test"); err != nil {
		t.Fatalf("InvalidateKeyed without reason: %v", err)
	}

	if err := e.tryInsertFact(t, e.nsID, kafkaClaim); !errors.Is(err, ErrRejectedValue) {
		t.Errorf("claim invalidated with a reason: err = %v; want ErrRejectedValue", err)
	}
	if err := e.tryInsertFact(t, e.nsID, without); err != nil {
		t.Errorf("claim invalidated without a reason: err = %v; want it stored (no judgement was recorded)", err)
	}
}
