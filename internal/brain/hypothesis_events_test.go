package brain

import (
	"context"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/scope"
)

// The actor decides who gets blamed for a change, so the precedence is the
// whole point: an explicit actor (the consolidator) must win over the token,
// and a token must win over the "system" fallback.
func TestActorFrom(t *testing.T) {
	ctx := context.Background()

	if got := actorFrom(ctx); got != "system" {
		t.Fatalf("no principal: got %q, want system", got)
	}

	ctx = scope.With(ctx, scope.Principal{Name: "agent"})
	if got := actorFrom(ctx); got != "agent" {
		t.Fatalf("principal: got %q, want agent", got)
	}

	ctx = WithActor(ctx, ActorConsolidator)
	if got := actorFrom(ctx); got != ActorConsolidator {
		t.Fatalf("explicit actor: got %q, want %q", got, ActorConsolidator)
	}

	if got := actorFrom(WithActor(context.Background(), "")); got != "system" {
		t.Fatalf("empty actor must not shadow the fallback: got %q", got)
	}
}

// The list joins hypotheses and the count does not, so the same filter has to
// come out aliased one way and plain the other. Getting this wrong is a SQL
// error at runtime, not at compile time.
func TestEventWhere(t *testing.T) {
	where, args := eventWhere("", []int64{1, 2}, 0)
	if where != "namespace_id = ANY($1)" || len(args) != 1 {
		t.Fatalf("plain: got %q with %d args", where, len(args))
	}

	where, args = eventWhere("e.", []int64{1}, 7)
	if where != "e.namespace_id = ANY($1) AND e.hypothesis_id = $2" {
		t.Fatalf("aliased: got %q", where)
	}
	if len(args) != 2 || args[1].(int64) != 7 {
		t.Fatalf("aliased args: got %#v", args)
	}
}
