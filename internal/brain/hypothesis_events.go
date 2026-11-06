// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/Bojanni050/cortexiq/internal/scope"
	"github.com/jackc/pgx/v5/pgconn"
)

// Event names in the hypothesis log. They are the vocabulary of the dashboard
// tab and the MCP tool, so they stay stable.
const (
	HypothesisEventProposed  = "proposed"
	HypothesisEventTesting   = "testing"
	HypothesisEventRefined   = "refined"
	HypothesisEventEvidence  = "evidence"
	HypothesisEventCounter   = "counter"
	HypothesisEventConfirmed = "confirmed"
	HypothesisEventRejected  = "rejected"
	HypothesisEventDeleted   = "deleted"
)

// ActorConsolidator is the actor for changes the background sweep makes on its
// own. Everything else is named after the principal that asked for it.
const ActorConsolidator = "consolidator"

type actorKey struct{}

// WithActor marks who is responsible for the changes made under ctx. The
// consolidator sets this once; anything coming through MCP or the dashboard is
// named after the token that authenticated the call, so no handler has to
// remember to pass it down.
func WithActor(ctx context.Context, actor string) context.Context {
	if actor == "" {
		return ctx
	}
	return context.WithValue(ctx, actorKey{}, actor)
}

// actorFrom resolves the actor for a log entry. A missing principal is honest
// rather than a guess: system calls (stdio CLI, migrations) are not a person.
func actorFrom(ctx context.Context) string {
	if actor, ok := ctx.Value(actorKey{}).(string); ok && actor != "" {
		return actor
	}
	if p, ok := scope.From(ctx); ok && p.Name != "" {
		return p.Name
	}
	return "system"
}

// execer is the small slice of pgx that recording an event needs, so the same
// code writes inside a transaction (confirm) and outside one (the rest).
type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// hypothesisEvent is one state change waiting to be written.
type hypothesisEvent struct {
	HypothesisID   int64
	NamespaceID    int64
	Event          string
	StatusFrom     *string
	StatusTo       string
	ConfidenceFrom *float32
	ConfidenceTo   *float32
	Reason         *string
	FactID         *int64
	Detail         any
}

func recordHypothesisEvent(ctx context.Context, db execer, ev hypothesisEvent) error {
	var detail []byte
	if ev.Detail != nil {
		b, err := json.Marshal(ev.Detail)
		if err != nil {
			return fmt.Errorf("marshal hypothesis event detail: %w", err)
		}
		detail = b
	}
	_, err := db.Exec(ctx,
		`INSERT INTO hypothesis_events
		 (hypothesis_id, namespace_id, event, actor, status_from, status_to,
		  confidence_from, confidence_to, reason, fact_id, detail)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		ev.HypothesisID, ev.NamespaceID, ev.Event, actorFrom(ctx), ev.StatusFrom, ev.StatusTo,
		ev.ConfidenceFrom, ev.ConfidenceTo, ev.Reason, ev.FactID, detail,
	)
	if err != nil {
		return fmt.Errorf("record hypothesis event: %w", err)
	}
	return nil
}

// eventWhere builds the filter the list and the count share. prefix is the
// table alias ("e." for the joined list query, empty for the count).
func eventWhere(prefix string, nsIDs []int64, hypothesisID int64) (string, []any) {
	where := prefix + "namespace_id = ANY($1)"
	args := []any{nsIDs}
	if hypothesisID > 0 {
		args = append(args, hypothesisID)
		where += fmt.Sprintf(" AND %shypothesis_id = $%d", prefix, len(args))
	}
	return where, args
}

// ListHypothesisEvents returns the log newest first. An unknown namespace is an
// empty log, not an error, which is what the dashboard expects while a slug is
// still being typed.
func (b *Brain) ListHypothesisEvents(ctx context.Context, namespaceSlugs []string, hypothesisID int64, page Pagination) ([]models.HypothesisEvent, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}
	page = page.Sanitize()

	where, args := eventWhere("e.", nsIDs, hypothesisID)
	query := `SELECT e.id, e.hypothesis_id, e.namespace_id, e.event, e.actor, e.status_from, e.status_to,
	          e.confidence_from, e.confidence_to, e.reason, e.fact_id, e.detail, e.created_at,
	          COALESCE(h.content, '')
	          FROM hypothesis_events e LEFT JOIN hypotheses h ON h.id = e.hypothesis_id
	          WHERE ` + where +
		fmt.Sprintf(" ORDER BY e.created_at DESC, e.id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, page.Limit, page.Offset)

	rows, err := b.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list hypothesis events: %w", err)
	}
	defer rows.Close()

	var result []models.HypothesisEvent
	for rows.Next() {
		var e models.HypothesisEvent
		if err := rows.Scan(&e.ID, &e.HypothesisID, &e.NamespaceID, &e.Event, &e.Actor,
			&e.StatusFrom, &e.StatusTo, &e.ConfidenceFrom, &e.ConfidenceTo,
			&e.Reason, &e.FactID, &e.Detail, &e.CreatedAt, &e.HypothesisContent); err != nil {
			return nil, fmt.Errorf("scan hypothesis event: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// CountHypothesisEvents counts what ListHypothesisEvents would return for the
// same filter, ignoring paging.
func (b *Brain) CountHypothesisEvents(ctx context.Context, namespaceSlugs []string, hypothesisID int64) (int, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return 0, err
	}
	where, args := eventWhere("", nsIDs, hypothesisID)
	return b.countWhere(ctx, "hypothesis_events", where, args...)
}

// strPtr is a small helper for the nullable text columns in a log entry.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
