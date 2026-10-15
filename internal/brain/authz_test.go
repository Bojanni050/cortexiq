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
