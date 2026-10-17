// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"errors"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/scope"
)

func TestValidateRename(t *testing.T) {
	anyErr := errors.New("any error")

	tests := []struct {
		name string
		from string
		to   string
		want error // sentinel matched with errors.Is; nil means no error expected
	}{
		{"moves a project namespace", "/projects/stash", "/projects/cortexiq", nil},
		{"allows a new parent path", "/a", "/b/c", nil},
		{"refuses the root as source", "/", "/x", ErrRenameRoot},
		{"refuses the root as target", "/x", "/", ErrRenameRoot},
		{"refuses the same path", "/a", "/a", anyErr},
		{"refuses a descendant as target", "/a", "/a/b", ErrRenameIntoDescendant},
		{"refuses a source without leading slash", "projects/stash", "/x", ErrInvalidPath},
		{"refuses a target without leading slash", "/a", "x", ErrInvalidPath},
		{"refuses an uppercase segment", "/Proj", "/x", ErrInvalidPath},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRename(tc.from, tc.to)
			switch {
			case tc.want == nil && err != nil:
				t.Fatalf("validateRename(%q, %q) = %v, want nil", tc.from, tc.to, err)
			case tc.want == anyErr && err == nil:
				t.Fatalf("validateRename(%q, %q) = nil, want an error", tc.from, tc.to)
			case tc.want != nil && tc.want != anyErr && !errors.Is(err, tc.want):
				t.Fatalf("validateRename(%q, %q) = %v, want %v", tc.from, tc.to, err, tc.want)
			}
		})
	}
}

func TestParentPaths(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"/a", nil},
		{"/a/b", []string{"/a"}},
		{"/a/b/c", []string{"/a", "/a/b"}},
	}

	for _, tc := range tests {
		got := parentPaths(tc.path)
		if len(got) != len(tc.want) {
			t.Fatalf("parentPaths(%q) = %v, want %v", tc.path, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("parentPaths(%q) = %v, want %v", tc.path, got, tc.want)
			}
		}
	}
}

// TestIntegrationRenameNamespace renames a namespace that has a child and a
// fact, and checks that both followed the path while nothing else moved.
func TestIntegrationRenameNamespace(t *testing.T) {
	e := newEnv(t, nil)

	factID := e.insertFact(t, "a fact that must survive a rename", nil)
	child := e.ns + "/child"
	if _, err := e.b.CreateNamespace(e.ctx, child, "Child", ""); err != nil {
		t.Fatalf("CreateNamespace(%q): %v", child, err)
	}

	moved := e.ns + "-renamed"
	res, err := e.b.RenameNamespace(e.ctx, e.ns, moved)
	if err != nil {
		t.Fatalf("RenameNamespace(%q, %q): %v", e.ns, moved, err)
	}
	if res.ID != e.nsID || res.From != e.ns || res.To != moved || res.Descendants != 1 {
		t.Fatalf("RenameNamespace = %+v, want id %d, from %q, to %q, 1 descendant", res, e.nsID, e.ns, moved)
	}

	if _, err := e.b.GetNamespace(e.ctx, e.ns); !errors.Is(err, ErrNamespaceNotFound) {
		t.Fatalf("old path %q still resolves: %v", e.ns, err)
	}
	renamed, err := e.b.GetNamespace(e.ctx, moved)
	if err != nil {
		t.Fatalf("GetNamespace(%q): %v", moved, err)
	}
	if renamed.ID != e.nsID {
		t.Fatalf("renamed namespace has id %d, want %d", renamed.ID, e.nsID)
	}
	// newEnv names the namespace after its own slug, so the name follows the path.
	if renamed.Name != moved {
		t.Fatalf("name = %q, want %q (name still equal to the old slug follows it)", renamed.Name, moved)
	}
	if _, err := e.b.GetNamespace(e.ctx, moved+"/child"); err != nil {
		t.Fatalf("descendant did not follow the rename: %v", err)
	}

	var facts int
	if err := e.b.pool.QueryRow(e.ctx,
		"SELECT count(*) FROM facts WHERE id = $1 AND namespace_id = $2", factID, res.ID,
	).Scan(&facts); err != nil {
		t.Fatalf("count facts: %v", err)
	}
	if facts != 1 {
		t.Fatalf("fact did not stay with the namespace: found %d", facts)
	}

	// The moved namespace keeps counting as itself for a recall-style lookup.
	ids, err := e.b.ResolveNamespaceIDs(e.ctx, []string{moved})
	if err != nil {
		t.Fatalf("ResolveNamespaceIDs(%q): %v", moved, err)
	}
	if len(ids) != 2 {
		t.Fatalf("ResolveNamespaceIDs(%q) = %v, want the namespace and its child", moved, ids)
	}

	t.Run("refuses a descendant as target", func(t *testing.T) {
		if _, err := e.b.RenameNamespace(e.ctx, moved, moved+"/child"); !errors.Is(err, ErrRenameIntoDescendant) {
			t.Fatalf("rename into descendant = %v, want ErrRenameIntoDescendant", err)
		}
	})

	t.Run("refuses a taken target", func(t *testing.T) {
		if _, err := e.b.RenameNamespace(e.ctx, moved+"/child", moved); !errors.Is(err, ErrNamespaceExists) {
			t.Fatalf("rename onto an existing path = %v, want ErrNamespaceExists", err)
		}
	})

	t.Run("reports an unknown source", func(t *testing.T) {
		if _, err := e.b.RenameNamespace(e.ctx, moved+"/missing", moved+"-x"); !errors.Is(err, ErrNamespaceNotFound) {
			t.Fatalf("rename of an unknown path = %v, want ErrNamespaceNotFound", err)
		}
	})

	t.Run("refuses a read-only token", func(t *testing.T) {
		ro := scope.With(e.ctx, scope.Principal{Name: "ro", ReadOnly: true})
		if _, err := e.b.RenameNamespace(ro, moved, moved+"-ro"); !errors.Is(err, ErrReadOnly) {
			t.Fatalf("rename with a read-only token = %v, want ErrReadOnly", err)
		}
	})
}
