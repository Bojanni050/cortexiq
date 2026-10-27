package brain

import (
	"context"
	"errors"

	"github.com/Bojanni050/cortexiq/internal/scope"
)

// ErrReadOnly means the token may only read.
var ErrReadOnly = errors.New("brain: token is read-only")

// NamespaceIDForWrite resolves the namespace a row is filed under, applying the
// caller's token default when the path is empty or "/". Write tools that take a
// namespace path use it instead of ResolveNamespaceIDs: that one expands "/" to
// every namespace, so taking its first element files the row under an arbitrary
// one — the exact mistake NamespaceIDByPath exists to prevent.
func (b *Brain) NamespaceIDForWrite(ctx context.Context, path string) (int64, error) {
	return b.NamespaceIDByPath(ctx, writeNamespace(ctx, path))
}

// requireWrite rejects mutating operations for read-only tokens. System calls
// (background consolidation, decay) carry no principal and are always allowed.
func requireWrite(ctx context.Context) error {
	if scope.ReadOnly(ctx) {
		return ErrReadOnly
	}
	return nil
}

// writeNamespace applies the caller's default namespace to a write that did not
// name one. The MCP remember tool defaults to "/", so an app that leaves the
// parameter out used to file everything at the root without anyone noticing; a
// token bound to a namespace turns that silent mis-filing into the right one.
//
// An explicit path wins. The binding is a default, not an allowlist — an app
// that knows it is writing somewhere else keeps the choice, and a token with no
// namespace changes nothing.
func writeNamespace(ctx context.Context, slug string) string {
	if slug != "" && slug != "/" {
		return slug
	}
	if ns := scope.Namespace(ctx); ns != "" {
		return ns
	}
	return slug
}
