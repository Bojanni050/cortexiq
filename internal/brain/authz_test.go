package brain

import (
	"context"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/scope"
)

func TestAuthorizeHelpers(t *testing.T) {
	// System calls (background consolidation, decay) carry no principal.
	if err := requireWrite(context.Background()); err != nil {
		t.Fatalf("system call must be allowed to write: %v", err)
	}

	readOnly := scope.With(context.Background(), scope.Principal{Name: "ro", ReadOnly: true})
	if err := requireWrite(readOnly); err != ErrReadOnly {
		t.Fatalf("requireWrite(read-only) = %v, want ErrReadOnly", err)
	}

	restricted := scope.With(context.Background(), scope.Principal{Name: "p", Namespaces: []string{"/projects"}})
	if err := checkReadAllowed(restricted, "/clients"); err != ErrForbidden {
		t.Fatalf("checkReadAllowed(/clients) = %v, want ErrForbidden", err)
	}
	if err := checkReadAllowed(restricted, "/projects/gaia"); err != nil {
		t.Fatalf("allowed path rejected: %v", err)
	}

	slugs, err := withAllowedSlugs(restricted, nil)
	if err != nil {
		t.Fatalf("withAllowedSlugs(nil): %v", err)
	}
	if len(slugs) != 1 || slugs[0] != "/projects" {
		t.Fatalf("withAllowedSlugs(nil) = %v, want [/projects]", slugs)
	}
	if _, err := withAllowedSlugs(restricted, []string{"/clients"}); err != ErrForbidden {
		t.Fatalf("withAllowedSlugs(/clients) = %v, want ErrForbidden", err)
	}
}
