package brain

import (
	"errors"
	"testing"
)

// Dismissing a relation hides it from the graph and the Clean-up report without
// deleting the row, and a second dismissal says there is nothing left to remove.
func TestIntegrationDismissRelationshipHidesButKeepsTheRow(t *testing.T) {
	e := newEnv(t, nil)
	var id int64
	if err := e.b.pool.QueryRow(e.ctx,
		`INSERT INTO relationships (namespace_id, from_entity, relation_type, to_entity, confidence)
		 VALUES ($1, 'Alpha Service', 'uses', 'Beta Queue', 0.4) RETURNING id`, e.nsID).Scan(&id); err != nil {
		t.Fatalf("insert relation: %v", err)
	}

	if err := e.b.DismissRelationship(e.ctx, id); err != nil {
		t.Fatalf("dismiss: %v", err)
	}

	var live, total int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*) FILTER (WHERE deleted_at IS NULL), count(*) FROM relationships WHERE id = $1`, id,
	).Scan(&live, &total); err != nil {
		t.Fatalf("count: %v", err)
	}
	if live != 0 || total != 1 {
		t.Fatalf("want the row kept but hidden, got live=%d total=%d", live, total)
	}

	rep, err := e.b.Cleanup(e.ctx, []string{e.ns}, CleanupOptions{MinConfidence: 0.8})
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	for _, w := range rep.Weak {
		if w.ID == id {
			t.Fatalf("a dismissed relation must not be reported as weak")
		}
	}

	if err := e.b.DismissRelationship(e.ctx, id); !errors.Is(err, ErrRelationshipNotFound) {
		t.Fatalf("second dismiss: want ErrRelationshipNotFound, got %v", err)
	}
	if err := e.b.DismissRelationship(e.ctx, id+9999); !errors.Is(err, ErrRelationshipNotFound) {
		t.Fatalf("unknown id: want ErrRelationshipNotFound, got %v", err)
	}
}
