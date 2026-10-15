package brain

import (
	"context"
	"errors"

	"github.com/Bojanni050/cortexiq/internal/scope"
)

var (
	// ErrForbidden means the token may not access the requested namespace.
	ErrForbidden = errors.New("brain: token is not allowed to access this namespace")
	// ErrReadOnly means the token may only read.
	ErrReadOnly = errors.New("brain: token is read-only")
)

// checkReadAllowed enforces the principal's namespace allowlist for a path.
func checkReadAllowed(ctx context.Context, path string) error {
	if !scope.PathAllowed(ctx, path) {
		return ErrForbidden
	}
	return nil
}

// requireWrite rejects mutating operations for read-only tokens.
func requireWrite(ctx context.Context) error {
	if scope.ReadOnly(ctx) {
		return ErrReadOnly
	}
	return nil
}

// withAllowedSlugs fills in the principal's namespaces when the caller passed
// none, and rejects slugs outside the allowlist.
func withAllowedSlugs(ctx context.Context, slugs []string) ([]string, error) {
	p, ok := scope.From(ctx)
	if !ok || len(p.Namespaces) == 0 {
		return slugs, nil
	}
	if len(slugs) == 0 {
		return p.Namespaces, nil
	}
	for _, s := range slugs {
		if !scope.PathAllowed(ctx, s) {
			return nil, ErrForbidden
		}
	}
	return slugs, nil
}
