package brain

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pgvector/pgvector-go"
)

// Export format: JSON Lines. The first line is a header, every following line is
// one record. Embeddings are deliberately not exported — they depend on the
// embedding model, so Import recomputes them with whatever model is configured.
// That makes an export portable across embedding models and vector dimensions.
const (
	// exportFormat identifies CortexIQ export files.
	exportFormat  = "cortexiq-export"
	exportVersion = 1

	RecordEpisode = "episode"
	RecordFact    = "fact"

	// maxImportLine bounds a single JSONL line (content is capped at maxContentLen).
	maxImportLine = 1 << 20
	// maxImportErrors bounds how many per-line errors are reported back.
	maxImportErrors = 20
)

type exportHeader struct {
	Format     string    `json:"format"`
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exported_at"`
}

// ExportRecord is one exported episode or fact.
type ExportRecord struct {
	Type       string     `json:"type"`
	Namespace  string     `json:"namespace"`
	Content    string     `json:"content"`
	OccurredAt *time.Time `json:"occurred_at,omitempty"` // episodes
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	Importance *float32   `json:"importance,omitempty"`
	Confidence *float32   `json:"confidence,omitempty"` // facts
	Entity     *string    `json:"entity,omitempty"`
	Property   *string    `json:"property,omitempty"`
	Value      *string    `json:"value,omitempty"`
	ValidFrom  *time.Time `json:"valid_from,omitempty"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
	KnownUntil *time.Time `json:"known_until,omitempty"` // facts: end of the knowledge interval
}

// ExportResult counts what an export wrote.
type ExportResult struct {
	Episodes int `json:"episodes"`
	Facts    int `json:"facts"`
}

// ImportOptions controls an import.
type ImportOptions struct {
	// Types limits what is imported (RecordEpisode / RecordFact). Empty means both.
	Types []string
	// DryRun validates and counts without writing or embedding anything.
	DryRun bool
}

// ImportResult counts what an import did.
type ImportResult struct {
	Episodes int      `json:"episodes"`
	Facts    int      `json:"facts"`
	Skipped  int      `json:"skipped"` // already present (same namespace + content) or filtered out
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
}

// Export streams the non-deleted episodes and facts of the given namespaces
// (each path includes its descendants) to w. types limits what is written; empty
// means both episodes and facts.
func (b *Brain) Export(ctx context.Context, w io.Writer, namespaces []string, types []string) (ExportResult, error) {
	var res ExportResult
	want, err := wantedTypes(types)
	if err != nil {
		return res, err
	}
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaces)
	if err != nil {
		return res, err
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(exportHeader{Format: exportFormat, Version: exportVersion, ExportedAt: time.Now().UTC()}); err != nil {
		return res, err
	}

	if want[RecordEpisode] {
		rows, err := b.pool.Query(ctx,
			`SELECT n.slug, e.content, e.occurred_at, e.created_at, e.importance
			 FROM episodes e JOIN namespaces n ON n.id = e.namespace_id
			 WHERE e.namespace_id = ANY($1) AND e.deleted_at IS NULL
			 ORDER BY e.id`, nsIDs)
		if err != nil {
			return res, fmt.Errorf("export episodes: %w", err)
		}
		for rows.Next() {
			rec := ExportRecord{Type: RecordEpisode}
			var occurred, created time.Time
			var importance float32
			if err := rows.Scan(&rec.Namespace, &rec.Content, &occurred, &created, &importance); err != nil {
				rows.Close()
				return res, fmt.Errorf("scan episode: %w", err)
			}
			rec.OccurredAt, rec.CreatedAt, rec.Importance = &occurred, &created, &importance
			if err := enc.Encode(rec); err != nil {
				rows.Close()
				return res, err
			}
			res.Episodes++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return res, fmt.Errorf("export episodes: %w", err)
		}
	}

	if want[RecordFact] {
		rows, err := b.pool.Query(ctx,
			`SELECT n.slug, f.content, f.confidence, f.entity, f.property, f.value,
			        f.valid_from, f.valid_until, f.known_until, f.created_at, f.importance
			 FROM facts f JOIN namespaces n ON n.id = f.namespace_id
			 WHERE f.namespace_id = ANY($1) AND f.deleted_at IS NULL
			 ORDER BY f.id`, nsIDs)
		if err != nil {
			return res, fmt.Errorf("export facts: %w", err)
		}
		for rows.Next() {
			rec := ExportRecord{Type: RecordFact}
			var conf, importance float32
			var created time.Time
			if err := rows.Scan(&rec.Namespace, &rec.Content, &conf, &rec.Entity, &rec.Property, &rec.Value,
				&rec.ValidFrom, &rec.ValidUntil, &rec.KnownUntil, &created, &importance); err != nil {
				rows.Close()
				return res, fmt.Errorf("scan fact: %w", err)
			}
			rec.Confidence, rec.CreatedAt, rec.Importance = &conf, &created, &importance
			if err := enc.Encode(rec); err != nil {
				rows.Close()
				return res, err
			}
			res.Facts++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return res, fmt.Errorf("export facts: %w", err)
		}
	}
	return res, nil
}

// Import reads an export produced by Export and stores its records, recomputing
// embeddings with the configured model. Records whose namespace + content already
// exist are skipped, so importing the same file twice is harmless. Namespaces are
// created as needed. A bad line is reported and skipped, not fatal.
//
// Note: imported episodes are new episodes, so the next consolidation will read
// them again; importing both episodes and facts can therefore produce duplicate
// facts that consolidation then deduplicates. Use Types to import only one kind.
func (b *Brain) Import(ctx context.Context, r io.Reader, opts ImportOptions) (ImportResult, error) {
	if err := requireWrite(ctx); err != nil {
		return ImportResult{}, err
	}
	var res ImportResult
	want, err := wantedTypes(opts.Types)
	if err != nil {
		return res, err
	}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxImportLine)

	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return res, err
		}
		return res, errors.New("import: empty input")
	}
	if err := checkHeader(sc.Bytes()); err != nil {
		return res, err
	}

	nsCache := map[string]int64{}
	line := 1
	for sc.Scan() {
		line++
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}

		rec, err := decodeRecord([]byte(raw))
		if err == nil && !want[rec.Type] {
			res.Skipped++
			continue
		}
		if err != nil {
			res.fail(line, err)
			continue
		}
		if opts.DryRun {
			bump(&res, rec.Type)
			continue
		}

		inserted, err := b.importRecord(ctx, rec, nsCache)
		switch {
		case err != nil:
			res.fail(line, err)
		case !inserted:
			res.Skipped++
		default:
			bump(&res, rec.Type)
		}
	}
	if err := sc.Err(); err != nil {
		return res, fmt.Errorf("import: read: %w", err)
	}
	return res, nil
}

func (r *ImportResult) fail(line int, err error) {
	r.Failed++
	if len(r.Errors) < maxImportErrors {
		r.Errors = append(r.Errors, fmt.Sprintf("line %d: %v", line, err))
	}
}

func bump(r *ImportResult, typ string) {
	if typ == RecordEpisode {
		r.Episodes++
	} else {
		r.Facts++
	}
}

func (b *Brain) importRecord(ctx context.Context, rec ExportRecord, nsCache map[string]int64) (bool, error) {
	nsID, ok := nsCache[rec.Namespace]
	if !ok {
		ns, err := b.GetNamespace(ctx, rec.Namespace)
		switch {
		case err == nil:
			nsID = ns.ID
		case errors.Is(err, ErrNamespaceNotFound):
			if nsID, err = b.CreateNamespace(ctx, rec.Namespace, rec.Namespace, ""); err != nil {
				return false, err
			}
		default:
			return false, err
		}
		nsCache[rec.Namespace] = nsID
	}

	table := "episodes"
	if rec.Type == RecordFact {
		table = "facts"
	}
	var exists bool
	if err := b.pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM "+table+" WHERE namespace_id = $1 AND content = $2 AND deleted_at IS NULL)",
		nsID, rec.Content,
	).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}

	vec, err := b.embedder.Embed(ctx, rec.Content)
	if err != nil {
		return false, fmt.Errorf("embed: %w", err)
	}
	pgVec := pgvector.NewVector(vec)
	now := time.Now().UTC()
	created := now
	if rec.CreatedAt != nil {
		created = rec.CreatedAt.UTC()
	}

	importance := DefaultImportance
	if rec.Importance != nil && *rec.Importance >= 0 && *rec.Importance <= 1 {
		importance = *rec.Importance
	}

	if rec.Type == RecordEpisode {
		occurred := created
		if rec.OccurredAt != nil {
			occurred = rec.OccurredAt.UTC()
		}
		_, err = b.pool.Exec(ctx,
			`INSERT INTO episodes (namespace_id, content, embedding, embedding_model, occurred_at, created_at, importance)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			nsID, rec.Content, pgVec, b.embedder.Model(), occurred, created, importance)
		return err == nil, err
	}

	conf := float32(1.0)
	if rec.Confidence != nil {
		conf = *rec.Confidence
	}
	_, err = b.pool.Exec(ctx,
		`INSERT INTO facts (namespace_id, content, embedding, embedding_model, confidence,
		                    entity, property, value, valid_from, valid_until, known_until, created_at, importance)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		nsID, rec.Content, pgVec, b.embedder.Model(), conf,
		rec.Entity, rec.Property, rec.Value, rec.ValidFrom, rec.ValidUntil, rec.KnownUntil, created, importance)
	return err == nil, err
}

func checkHeader(line []byte) error {
	var h exportHeader
	if err := json.Unmarshal(line, &h); err != nil {
		return fmt.Errorf("import: not a %s file (missing header line)", exportFormat)
	}
	if h.Format != exportFormat {
		return fmt.Errorf("import: not a %s file (missing header line)", exportFormat)
	}
	if h.Version != exportVersion {
		return fmt.Errorf("import: unsupported export version %d (this build reads version %d)", h.Version, exportVersion)
	}
	return nil
}

// decodeRecord parses and validates one record line.
func decodeRecord(line []byte) (ExportRecord, error) {
	var rec ExportRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return rec, fmt.Errorf("invalid JSON: %w", err)
	}
	if rec.Type != RecordEpisode && rec.Type != RecordFact {
		return rec, fmt.Errorf("unknown record type %q", rec.Type)
	}
	if err := validatePath(rec.Namespace); err != nil {
		return rec, err
	}
	if strings.TrimSpace(rec.Content) == "" {
		return rec, ErrEmptyContent
	}
	if err := validateContent(rec.Content); err != nil {
		return rec, err
	}
	if rec.Confidence != nil && (*rec.Confidence < 0 || *rec.Confidence > 1) {
		return rec, fmt.Errorf("confidence %v out of range 0..1", *rec.Confidence)
	}
	return rec, nil
}

func wantedTypes(types []string) (map[string]bool, error) {
	want := map[string]bool{}
	if len(types) == 0 {
		want[RecordEpisode], want[RecordFact] = true, true
		return want, nil
	}
	for _, t := range types {
		t = strings.TrimSpace(strings.ToLower(t))
		if t != RecordEpisode && t != RecordFact {
			return nil, fmt.Errorf("unknown type %q (use %s or %s)", t, RecordEpisode, RecordFact)
		}
		want[t] = true
	}
	return want, nil
}
