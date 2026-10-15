package brain

import (
	"context"
	"errors"

	"github.com/Bojanni050/cortexiq/internal/scope"
)

// ErrReadOnly means the token may only read.
var ErrReadOnly = errors.New("brain: token is read-only")

// requireWrite rejects mutating operations for read-only tokens. System calls
// (background consolidation, decay) carry no principal and are always allowed.
func requireWrite(ctx context.Context) error {
	if scope.ReadOnly(ctx) {
		return ErrReadOnly
	}
	return nil
}
