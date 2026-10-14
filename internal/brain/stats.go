package brain

import (
	"context"
	"fmt"
	"strings"
)

// StatsTable is the row count and on-disk size of a single table.
type StatsTable struct {
	Name  string `json:"name"`
	Rows  int64  `json:"rows"`
	Bytes int64  `json:"size_bytes"`
}

// Stats is a read-only snapshot of what the memory store holds: how much is
// stored per table, the total database size, and the date range of the oldest
// and newest stored memory.
type Stats struct {
	DatabaseBytes int64        `json:"database_bytes"`
	EntryCount    int64        `json:"entry_count"`
	Oldest        *string      `json:"oldest,omitempty"`
	Newest        *string      `json:"newest,omitempty"`
	Tables        []StatsTable `json:"tables"`
}

// statsCountTables lists the tables we report, in display order. The names are
// compile-time constants and are only ever used to build SQL identifiers.
var statsCountTables = []string{
	"facts", "episodes", "patterns", "relationships",
	"causal_links", "contradictions", "goals", "hypotheses",
	"failures", "namespaces", "contexts", "consolidation_runs",
	"embedding_cache",
}

// entryTables hold actual stored memories; their rows make up EntryCount.
var entryTables = map[string]bool{
	"facts": true, "episodes": true, "patterns": true, "relationships": true,
	"causal_links": true, "contradictions": true, "goals": true,
	"hypotheses": true, "failures": true,
}

// Stats gathers memory statistics for the dashboard and the memory_stats tool.
func (b *Brain) Stats(ctx context.Context) (Stats, error) {
	var st Stats

	if err := b.pool.QueryRow(ctx,
		`SELECT pg_database_size(current_database())`).Scan(&st.DatabaseBytes); err != nil {
		return st, err
	}

	sizes, err := b.tableSizes(ctx)
	if err != nil {
		return st, err
	}

	counts, err := b.tableCounts(ctx)
	if err != nil {
		return st, err
	}

	st.Tables = make([]StatsTable, 0, len(statsCountTables))
	for _, name := range statsCountTables {
		rows := counts[name]
		st.Tables = append(st.Tables, StatsTable{Name: name, Rows: rows, Bytes: sizes[name]})
		if entryTables[name] {
			st.EntryCount += rows
		}
	}

	var oldest, newest *string
	if err := b.pool.QueryRow(ctx, `
		SELECT min(created_at)::text, max(created_at)::text
		FROM (SELECT created_at FROM facts UNION ALL SELECT created_at FROM episodes) m`,
	).Scan(&oldest, &newest); err != nil {
		return st, err
	}
	st.Oldest = oldest
	st.Newest = newest

	return st, nil
}

func (b *Brain) tableSizes(ctx context.Context) (map[string]int64, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT c.relname, pg_total_relation_size(c.oid)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sizes := make(map[string]int64)
	for rows.Next() {
		var name string
		var bytes int64
		if err := rows.Scan(&name, &bytes); err != nil {
			return nil, err
		}
		sizes[name] = bytes
	}
	return sizes, rows.Err()
}

func (b *Brain) tableCounts(ctx context.Context) (map[string]int64, error) {
	var q strings.Builder
	for i, name := range statsCountTables {
		if i > 0 {
			q.WriteString(" UNION ALL ")
		}
		fmt.Fprintf(&q, "SELECT '%s' AS name, count(*) AS n FROM %s", name, name)
	}

	rows, err := b.pool.Query(ctx, q.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var name string
		var n int64
		if err := rows.Scan(&name, &n); err != nil {
			return nil, err
		}
		counts[name] = n
	}
	return counts, rows.Err()
}
