package brain

import "testing"

// The root namespace is what a client that names no namespace writes to (the MCP
// remember tool defaults to "/"). Migration 00036 creates it, so a write there
// must land like any other instead of failing with ErrNamespaceNotFound.
func TestIntegrationRememberInRootNamespace(t *testing.T) {
	e := newEnv(t, nil)

	id, err := e.b.Remember(e.ctx, "/", "an uncategorized remark", nil)
	if err != nil {
		t.Fatalf("Remember(/) = %v, want a stored episode", err)
	}

	var slug string
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT n.slug FROM episodes ep JOIN namespaces n ON n.id = ep.namespace_id WHERE ep.id = $1`, id,
	).Scan(&slug); err != nil {
		t.Fatal(err)
	}
	if slug != "/" {
		t.Errorf("episode filed under %q, want /", slug)
	}

	// A "/" read keeps meaning "every namespace", so the root row must not
	// change what recall resolves.
	ids, err := e.b.ResolveNamespaceIDs(e.ctx, []string{"/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) < 2 {
		t.Errorf("ResolveNamespaceIDs(/) = %v, want every namespace", ids)
	}
}
