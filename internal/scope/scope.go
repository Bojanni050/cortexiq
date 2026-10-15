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
