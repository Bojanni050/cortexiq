package brain

import (
	"context"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/scope"
)

func TestRequireWrite(t *testing.T) {
	// System calls (background consolidation, decay) carry no principal.
	if err := requireWrite(context.Background()); err != nil {
		t.Fatalf("system call must be allowed to write: %v", err)
	}

	readWrite := scope.With(context.Background(), scope.Principal{Name: "rw"})
	if err := requireWrite(readWrite); err != nil {
		t.Fatalf("read-write token must be allowed to write: %v", err)
	}

	readOnly := scope.With(context.Background(), scope.Principal{Name: "ro", ReadOnly: true})
	if err := requireWrite(readOnly); err != ErrReadOnly {
		t.Fatalf("requireWrite(read-only) = %v, want ErrReadOnly", err)
	}
}

// A token bound to a namespace answers "where does this go" when the app does
// not, which is the common case: the MCP tool's own default is "/", so the
// parameter is usually simply absent.
func TestWriteNamespace(t *testing.T) {
	tests := []struct {
		name      string
		principal *scope.Principal
		slug      string
		want      string
	}{
		{"no principal, root", nil, "/", "/"},
		{"no principal, empty", nil, "", ""},
		{"no principal, explicit", nil, "/projects/x", "/projects/x"},
		{"bound, root becomes the binding", &scope.Principal{Name: "a", Namespace: "/projects/x"}, "/", "/projects/x"},
		{"bound, empty becomes the binding", &scope.Principal{Name: "a", Namespace: "/projects/x"}, "", "/projects/x"},
		{"bound, explicit path wins", &scope.Principal{Name: "a", Namespace: "/projects/x"}, "/projects/y", "/projects/y"},
		{"unbound principal keeps root", &scope.Principal{Name: "a"}, "/", "/"},
	}
	for _, tt := range tests {
		ctx := context.Background()
		if tt.principal != nil {
			ctx = scope.With(ctx, *tt.principal)
		}
		if got := writeNamespace(ctx, tt.slug); got != tt.want {
			t.Errorf("%s: writeNamespace(%q) = %q, want %q", tt.name, tt.slug, got, tt.want)
		}
	}
}
