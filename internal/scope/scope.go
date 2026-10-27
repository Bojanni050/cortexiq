// Package scope carries the authenticated principal (the token's read/write
// scope) through a request context so the brain can enforce it uniformly for the
// dashboard API and the MCP server.
package scope

import (
	"context"
)

// Principal is the authenticated caller for one request.
type Principal struct {
	Name     string
	ReadOnly bool
	// Namespace is where this caller's writes belong when it does not name one.
	// It is a default, not a restriction: a write that names a path keeps it.
	// See brain.writeNamespace for why it exists.
	Namespace string
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

// Namespace returns the namespace this caller's writes default to, or "" when
// the caller has none (a CLI call, or a token without one).
func Namespace(ctx context.Context) string {
	p, _ := From(ctx)
	return p.Namespace
}
