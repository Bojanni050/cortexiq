// Package scope carries the authenticated principal (token scope and namespace
// allowlist) through a request context so the brain can enforce it uniformly for
// the dashboard API and the MCP server.
package scope

import (
	"context"
	"strings"
)

// Principal is the authenticated caller for one request.
type Principal struct {
	Name     string
	ReadOnly bool
	// Namespaces is the allowlist of namespace paths. Empty means every
	// namespace is allowed. "/" means the whole tree.
	Namespaces []string
}

type ctxKey struct{}

// With attaches p to ctx. Internal (system) calls carry no principal at all,
// which is what distinguishes them from an authenticated request.
func With(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// From returns the request principal, if any.
func From(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// ReadOnly reports whether the caller may only read.
func ReadOnly(ctx context.Context) bool {
	p, _ := From(ctx)
	return p.ReadOnly
}

// Restricted reports whether the principal has a namespace allowlist.
func Restricted(ctx context.Context) bool {
	p, ok := From(ctx)
	return ok && len(p.Namespaces) > 0
}

// PathAllowed reports whether path is inside the principal's allowlist. A
// principal without namespaces may access everything.
func PathAllowed(ctx context.Context, path string) bool {
	p, ok := From(ctx)
	if !ok || len(p.Namespaces) == 0 {
		return true
	}
	for _, allowed := range p.Namespaces {
		if allowed == "/" {
			return true
		}
		allowed = strings.TrimSuffix(allowed, "/")
		if path == allowed || path == allowed+"/" || strings.HasPrefix(path, allowed+"/") {
			return true
		}
	}
	return false
}
