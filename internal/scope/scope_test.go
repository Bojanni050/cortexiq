package scope

import (
	"context"
	"testing"
)

func TestReadOnly(t *testing.T) {
	ctx := With(context.Background(), Principal{Name: "ro", ReadOnly: true})
	if !ReadOnly(ctx) {
		t.Fatal("expected read-only")
	}
	if ReadOnly(context.Background()) {
		t.Fatal("a bare context must not be read-only")
	}
}

func TestFrom(t *testing.T) {
	p, ok := From(With(context.Background(), Principal{Name: "dashboard"}))
	if !ok || p.Name != "dashboard" || p.ReadOnly {
		t.Fatalf("From() = %+v, ok=%v", p, ok)
	}
	if _, ok := From(context.Background()); ok {
		t.Fatal("a bare context must carry no principal")
	}
}
