package scope

import (
	"context"
	"testing"
)

func TestPathAllowed(t *testing.T) {
	bare := context.Background()
	all := With(bare, Principal{Name: "all"})
	proj := With(bare, Principal{Name: "p", Namespaces: []string{"/projects"}})
	multi := With(bare, Principal{Name: "m", Namespaces: []string{"/projects", "/user"}})

	cases := []struct {
		name string
		ctx  context.Context
		path string
		want bool
	}{
		{"no principal is unrestricted", bare, "/anything", true},
		{"principal without namespaces is unrestricted", all, "/clients/x", true},
		{"exact match", proj, "/projects", true},
		{"descendant", proj, "/projects/gaia", true},
		{"sibling denied", proj, "/clients", false},
		{"root denied for restricted token", proj, "/", false},
		{"prefix lookalike denied", proj, "/projectsx", false},
		{"second allow entry", multi, "/user", true},
	}
	for _, c := range cases {
		if got := PathAllowed(c.ctx, c.path); got != c.want {
			t.Errorf("%s: PathAllowed(%q) = %v, want %v", c.name, c.path, got, c.want)
		}
	}
}

func TestReadOnlyAndRestricted(t *testing.T) {
	ctx := With(context.Background(), Principal{ReadOnly: true, Namespaces: []string{"/x"}})
	if !ReadOnly(ctx) {
		t.Fatal("expected read-only")
	}
	if !Restricted(ctx) {
		t.Fatal("expected restricted")
	}
	if ReadOnly(context.Background()) || Restricted(context.Background()) {
		t.Fatal("a bare context must be unrestricted")
	}
}
