// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Bojanni050/cortexiq/internal/hygiene"
	"github.com/jackc/pgx/v5"
)

// HygieneCandidate is one memory the auditor scored. It carries enough to act
// on (table, id, namespace) and enough to review (preview, reasons, score).
type HygieneCandidate struct {
	Table        string    `json:"table"` // "episodes" or "facts"
	ID           int64     `json:"id"`
	NamespaceID  int64     `json:"namespace_id"`
	Content      string    `json:"content"`
	Preview      string    `json:"preview"`
	Importance   float64   `json:"importance"`
	NoiseScore   float64   `json:"noise_score"`
	Reasons      []string  `json:"reasons"`
	SecretLabels []string  `json:"secret_labels,omitempty"`
	Suggested    string    `json:"suggested_action"`
	OccurredAt   time.Time `json:"occurred_at,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// HygieneAuditOptions controls one audit scan. Audit is always read-only.
type HygieneAuditOptions struct {
	// Namespaces limits the scan to these paths (each includes descendants).
	// Empty means all namespaces.
	Namespaces []string
	// MinScore is the lowest noise score to return (0..1). Candidates below it
	// are not reported. Secrets are always reported regardless of MinScore.
	MinScore float64
	// Limit caps how many candidates are returned.
	Limit int
	// Offset skips this many candidates (for pagination).
	Offset int
	// Tables restricts the scan to these tables ("episodes", "facts").
	// Empty means both.
	Tables []string
}

// HygieneAuditEntry is one row of the audit log.
type HygieneAuditEntry struct {
	ID                 int64     `json:"id"`
	NamespaceID        int64     `json:"namespace_id"`
	TargetTable        string    `json:"target_table"`
	TargetID           int64     `json:"target_id"`
	Action             string    `json:"action"`
	Reasons            []string  `json:"reasons"`
	NoiseScore         float64   `json:"noise_score"`
	SecretLabels       []string  `json:"secret_labels"`
	ContentPreview     string    `json:"content_preview"`
	OriginalImportance *float64  `json:"original_importance,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

// HygieneCleanOptions controls one cleaning run. Cleaning is a dry run unless
// Confirm is true.
type HygieneCleanOptions struct {
	// Action is what to do with each candidate: "archive", "flag", or "keep".
	Action string
	// Confirm must be true to actually write. Without it the run is a dry run
	// and reports what would happen.
	Confirm bool
	// Actor is recorded in the audit log as the one who ran the clean.
	Actor string
}

// HygieneCleanResult summarises one cleaning run.
type HygieneCleanResult struct {
	Action     string             `json:"action"`
	DryRun     bool               `json:"dry_run"`
	Candidates []HygieneCandidate `json:"candidates"`
	Counts     map[string]int     `json:"counts"`
}

// HygieneStatus summarises the audit log and the current archive/flag state.
type HygieneStatus struct {
	AuditLogCount    int                 `json:"audit_log_count"`
	ByAction         map[string]int      `json:"by_action"`
	ArchivedEpisodes int                 `json:"archived_episodes"`
	ArchivedFacts    int                 `json:"archived_facts"`
	FlaggedEpisodes  int                 `json:"flagged_episodes"`
	FlaggedFacts     int                 `json:"flagged_facts"`
	RecentEntries    []HygieneAuditEntry `json:"recent_entries,omitempty"`
}

const defaultHygieneLimit = 200
const maxHygieneLimit = 1000

// Audit scans memories for noise. It is read-only: nothing is written, not
// even to the audit log. The audit log is only written by Clean.
func (b *Brain) Audit(ctx context.Context, opts HygieneAuditOptions) ([]HygieneCandidate, error) {
	if opts.Limit <= 0 {
		opts.Limit = defaultHygieneLimit
	}
	if opts.Limit > maxHygieneLimit {
		opts.Limit = maxHygieneLimit
	}
	if opts.MinScore < 0 {
		opts.MinScore = 0
	}
	if opts.MinScore > 1 {
		opts.MinScore = 1
	}

	var nsIDs []int64
	if len(opts.Namespaces) > 0 {
		var err error
		nsIDs, err = b.resolveNamespaceIDs(ctx, opts.Namespaces)
		if err != nil {
			return nil, fmt.Errorf("hygiene audit: %w", err)
		}
	}

	tables := opts.Tables
	if len(tables) == 0 {
		tables = []string{"episodes", "facts"}
	}

	var candidates []HygieneCandidate
	for _, table := range tables {
		switch table {
		case "episodes", "facts":
		default:
			return nil, fmt.Errorf("hygiene audit: unknown table %q (use episodes or facts)", table)
		}
		cs, err := b.auditTable(ctx, table, nsIDs, opts)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, cs...)
	}

	// Sort by noise score descending so the worst noise is first.
	sortCandidatesByScore(candidates)

	if len(candidates) > opts.Limit {
		candidates = candidates[:opts.Limit]
	}
	if opts.Offset > 0 && opts.Offset < len(candidates) {
		candidates = candidates[opts.Offset:]
	}

	return candidates, nil
}

// auditTable scans one table and returns the candidates above MinScore.
func (b *Brain) auditTable(ctx context.Context, table string, nsIDs []int64, opts HygieneAuditOptions) ([]HygieneCandidate, error) {
	// Only scan live, non-archived, non-flagged rows: archived rows are already
	// out of ranking, flagged rows are already marked for review.
	query := fmt.Sprintf(`
		SELECT id, namespace_id, content, importance, occurred_at, created_at
		FROM %s
		WHERE deleted_at IS NULL
		  AND hygiene_archived = FALSE
		  AND hygiene_flagged = FALSE
	`, table)
	var args []any
	if len(nsIDs) > 0 {
		query += " AND namespace_id = ANY($1)"
		args = append(args, nsIDs)
	}
	// No ORDER BY: the scan is a full table read and the sort happens in Go
	// after scoring, because the score is computed in the application.
	query += " ORDER BY id"

	rows, err := b.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("hygiene audit %s: %w", table, err)
	}
	defer rows.Close()

	var out []HygieneCandidate
	for rows.Next() {
		var (
			id, nsID   int64
			content    string
			importance float64
			occurredAt time.Time
			createdAt  time.Time
		)
		if err := rows.Scan(&id, &nsID, &content, &importance, &occurredAt, &createdAt); err != nil {
			return nil, fmt.Errorf("hygiene audit %s: scan: %w", table, err)
		}
		// Episodes and facts have no "source" column; the source is the table
		// name for noise purposes (a heartbeat episode is noise, a heartbeat
		// fact is derived noise).
		source := ""
		s := hygiene.ScoreContent(content, importance, source)
		// Secrets always surface regardless of MinScore.
		if s.Value < opts.MinScore && len(s.SecretLabels) == 0 {
			continue
		}
		out = append(out, HygieneCandidate{
			Table:        table,
			ID:           id,
			NamespaceID:  nsID,
			Content:      content,
			Preview:      hygiene.Preview(content, 200),
			Importance:   importance,
			NoiseScore:   s.Value,
			Reasons:      s.Reasons,
			SecretLabels: s.SecretLabels,
			Suggested:    hygiene.SuggestedAction(s),
			OccurredAt:   occurredAt,
			CreatedAt:    createdAt,
		})
	}
	return out, rows.Err()
}

// Clean acts on candidates: archive (importance 0, reversible), flag (metadata
// only), or keep (log only). Without Confirm it is a dry run: nothing is
// written to the memory tables, and the audit log is not written either.
func (b *Brain) Clean(ctx context.Context, candidates []HygieneCandidate, opts HygieneCleanOptions) (HygieneCleanResult, error) {
	action := strings.ToLower(strings.TrimSpace(opts.Action))
	switch action {
	case "archive", "flag", "keep":
	default:
		return HygieneCleanResult{}, fmt.Errorf("hygiene clean: unknown action %q (use archive, flag or keep)", opts.Action)
	}
	actor := strings.TrimSpace(opts.Actor)
	if actor == "" {
		actor = "cli"
	}

	result := HygieneCleanResult{
		Action:     action,
		DryRun:     !opts.Confirm,
		Candidates: candidates,
		Counts:     map[string]int{"total": len(candidates)},
	}

	if !opts.Confirm {
		// Dry run: count what would happen, write nothing.
		for range candidates {
			result.Counts["would_"+action]++
		}
		return result, nil
	}

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return result, fmt.Errorf("hygiene clean: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, c := range candidates {
		switch action {
		case "archive":
			if err := b.archiveHygieneCandidate(ctx, tx, c); err != nil {
				return result, err
			}
			result.Counts["archived"]++
		case "flag":
			if err := b.flagHygieneCandidate(ctx, tx, c, strings.Join(c.Reasons, ", ")); err != nil {
				return result, err
			}
			result.Counts["flagged"]++
		case "keep":
			result.Counts["kept"]++
		}
		// Every candidate gets one audit log row, including kept ones, so a
		// run is reproducible.
		if err := b.logHygieneAction(ctx, tx, c, action, actor); err != nil {
			return result, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return result, fmt.Errorf("hygiene clean: commit: %w", err)
	}
	return result, nil
}

// archiveHygieneCandidate sets importance 0 and the archive markers on one
// memory. The original importance is preserved so restore is exact.
func (b *Brain) archiveHygieneCandidate(ctx context.Context, tx pgx.Tx, c HygieneCandidate) error {
	var query string
	switch c.Table {
	case "episodes":
		query = `UPDATE episodes SET
			importance = 0,
			hygiene_archived = TRUE,
			hygiene_archived_at = now(),
			hygiene_archive_reason = $1,
			hygiene_original_importance = importance
		 WHERE id = $2 AND deleted_at IS NULL`
	case "facts":
		query = `UPDATE facts SET
			importance = 0,
			hygiene_archived = TRUE,
			hygiene_archived_at = now(),
			hygiene_archive_reason = $1,
			hygiene_original_importance = importance
		 WHERE id = $2 AND deleted_at IS NULL`
	}
	reason := strings.Join(c.Reasons, ", ")
	if reason == "" {
		reason = "hygiene archive"
	}
	tag, err := tx.Exec(ctx, query, reason, c.ID)
	if err != nil {
		return fmt.Errorf("hygiene archive %s %d: %w", c.Table, c.ID, err)
	}
	if tag.RowsAffected() == 0 {
		// Already archived or deleted between audit and clean; not an error.
		return nil
	}
	return nil
}

// flagHygieneCandidate sets the flag markers on one memory. Content and
// importance are untouched.
func (b *Brain) flagHygieneCandidate(ctx context.Context, tx pgx.Tx, c HygieneCandidate, reason string) error {
	var query string
	switch c.Table {
	case "episodes":
		query = `UPDATE episodes SET hygiene_flagged = TRUE, hygiene_flag_reason = $1 WHERE id = $2 AND deleted_at IS NULL`
	case "facts":
		query = `UPDATE facts SET hygiene_flagged = TRUE, hygiene_flag_reason = $1 WHERE id = $2 AND deleted_at IS NULL`
	}
	if reason == "" {
		reason = "hygiene flag"
	}
	if _, err := tx.Exec(ctx, query, reason, c.ID); err != nil {
		return fmt.Errorf("hygiene flag %s %d: %w", c.Table, c.ID, err)
	}
	return nil
}

// logHygieneAction writes one row to hygiene_audit_log. The content preview is
// redacted so a secret can never leak into the log.
func (b *Brain) logHygieneAction(ctx context.Context, tx pgx.Tx, c HygieneCandidate, action, actor string) error {
	reasons := c.Reasons
	if len(reasons) == 0 {
		reasons = []string{"kept"}
	}
	secretLabels := c.SecretLabels
	if len(secretLabels) == 0 {
		secretLabels = []string{}
	}
	preview := hygiene.Preview(c.Content, 200)
	_, err := tx.Exec(ctx, `
		INSERT INTO hygiene_audit_log
			(namespace_id, target_table, target_id, action, reasons, noise_score, secret_labels, content_preview, original_importance)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		c.NamespaceID, c.Table, c.ID, action, reasons, c.NoiseScore, secretLabels, preview, c.Importance,
	)
	if err != nil {
		return fmt.Errorf("hygiene log %s %d: %w", c.Table, c.ID, err)
	}
	return nil
}

// Restore reverses archives: it puts the original importance back and clears
// the archive markers. Flagged rows are not touched (a flag is not undone by
// restore; it is cleared by an explicit unflag). Restore is always a real
// write — there is no dry-run variant because it is already the undo.
func (b *Brain) Restore(ctx context.Context, namespaces []string, limit int) (int, error) {
	if limit <= 0 {
		limit = defaultHygieneLimit
	}
	if limit > maxHygieneLimit {
		limit = maxHygieneLimit
	}

	var nsIDs []int64
	if len(namespaces) > 0 {
		var err error
		nsIDs, err = b.resolveNamespaceIDs(ctx, namespaces)
		if err != nil {
			return 0, fmt.Errorf("hygiene restore: %w", err)
		}
	}

	total := 0
	for _, table := range []string{"episodes", "facts"} {
		var query string
		var args []any
		switch table {
		case "episodes":
			query = `UPDATE episodes SET
				importance = COALESCE(hygiene_original_importance, 0.5),
				hygiene_archived = FALSE,
				hygiene_archived_at = NULL,
				hygiene_archive_reason = NULL,
				hygiene_original_importance = NULL
			 WHERE hygiene_archived = TRUE AND deleted_at IS NULL`
		case "facts":
			query = `UPDATE facts SET
				importance = COALESCE(hygiene_original_importance, 0.5),
				hygiene_archived = FALSE,
				hygiene_archived_at = NULL,
				hygiene_archive_reason = NULL,
				hygiene_original_importance = NULL
			 WHERE hygiene_archived = TRUE AND deleted_at IS NULL`
		}
		if len(nsIDs) > 0 {
			query += " AND namespace_id = ANY($1)"
			args = append(args, nsIDs)
		}
		query += fmt.Sprintf(" LIMIT %d", limit)

		tag, err := b.pool.Exec(ctx, query, args...)
		if err != nil {
			return total, fmt.Errorf("hygiene restore %s: %w", table, err)
		}
		total += int(tag.RowsAffected())
	}
	return total, nil
}

// Status summarises the audit log and the current archive/flag counts.
func (b *Brain) HygieneStatus(ctx context.Context, namespaces []string, limit int) (HygieneStatus, error) {
	var status HygieneStatus
	status.ByAction = map[string]int{}

	if limit <= 0 {
		limit = 20
	}

	var nsIDs []int64
	if len(namespaces) > 0 {
		var err error
		nsIDs, err = b.resolveNamespaceIDs(ctx, namespaces)
		if err != nil {
			return status, fmt.Errorf("hygiene status: %w", err)
		}
	}

	// Audit log count and per-action counts.
	countQuery := `SELECT count(*), COALESCE(array_agg(action), '{}') FROM hygiene_audit_log`
	countArgs := []any{}
	if len(nsIDs) > 0 {
		countQuery = `SELECT count(*), COALESCE(array_agg(action), '{}') FROM hygiene_audit_log WHERE namespace_id = ANY($1)`
		countArgs = append(countArgs, nsIDs)
	}
	var actions []string
	if err := b.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&status.AuditLogCount, &actions); err != nil {
		return status, fmt.Errorf("hygiene status: count: %w", err)
	}
	for _, a := range actions {
		status.ByAction[a]++
	}

	// Current archive/flag counts.
	if err := b.pool.QueryRow(ctx, `SELECT count(*) FROM episodes WHERE hygiene_archived = TRUE AND deleted_at IS NULL`).Scan(&status.ArchivedEpisodes); err != nil {
		return status, fmt.Errorf("hygiene status: archived episodes: %w", err)
	}
	if err := b.pool.QueryRow(ctx, `SELECT count(*) FROM facts WHERE hygiene_archived = TRUE AND deleted_at IS NULL`).Scan(&status.ArchivedFacts); err != nil {
		return status, fmt.Errorf("hygiene status: archived facts: %w", err)
	}
	if err := b.pool.QueryRow(ctx, `SELECT count(*) FROM episodes WHERE hygiene_flagged = TRUE AND deleted_at IS NULL`).Scan(&status.FlaggedEpisodes); err != nil {
		return status, fmt.Errorf("hygiene status: flagged episodes: %w", err)
	}
	if err := b.pool.QueryRow(ctx, `SELECT count(*) FROM facts WHERE hygiene_flagged = TRUE AND deleted_at IS NULL`).Scan(&status.FlaggedFacts); err != nil {
		return status, fmt.Errorf("hygiene status: flagged facts: %w", err)
	}

	// Recent audit entries.
	entryQuery := `SELECT id, namespace_id, target_table, target_id, action, reasons, noise_score, secret_labels, content_preview, original_importance, created_at FROM hygiene_audit_log`
	entryArgs := []any{}
	if len(nsIDs) > 0 {
		entryQuery += " WHERE namespace_id = ANY($1)"
		entryArgs = append(entryArgs, nsIDs)
	}
	entryQuery += fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d", limit)

	rows, err := b.pool.Query(ctx, entryQuery, entryArgs...)
	if err != nil {
		return status, fmt.Errorf("hygiene status: entries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e HygieneAuditEntry
		var origImp *float64
		if err := rows.Scan(&e.ID, &e.NamespaceID, &e.TargetTable, &e.TargetID, &e.Action, &e.Reasons, &e.NoiseScore, &e.SecretLabels, &e.ContentPreview, &origImp, &e.CreatedAt); err != nil {
			return status, fmt.Errorf("hygiene status: scan entry: %w", err)
		}
		e.OriginalImportance = origImp
		status.RecentEntries = append(status.RecentEntries, e)
	}
	return status, rows.Err()
}

// sortCandidatesByScore orders candidates by noise score descending, then by
// table and id for a stable order.
func sortCandidatesByScore(cs []HygieneCandidate) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0; j-- {
			a, b := cs[j-1], cs[j]
			if a.NoiseScore > b.NoiseScore ||
				(a.NoiseScore == b.NoiseScore && a.Table > b.Table) ||
				(a.NoiseScore == b.NoiseScore && a.Table == b.Table && a.ID > b.ID) {
				break
			}
			cs[j-1], cs[j] = cs[j], cs[j-1]
		}
	}
}
