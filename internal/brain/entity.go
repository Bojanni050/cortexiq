// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"strings"
)

// entityTrimWords describe how something is set up, deployed or configured rather
// than which thing it is. Trailing words like these are what turned one product
// into four entities in production:
//
//	hindsight · hindsight mcp setup · hindsight mcp setup in claude desktop ·
//	hindsight's mcp server
//
// The list is deliberately explicit instead of a similarity measure. "Hindsight"
// and "Hindsight Cloud" are two different products, and no string distance can
// tell that apart from "Hindsight" and "Hindsight MCP setup", which are one.
var entityTrimWords = map[string]bool{
	"setup": true, "integration": true, "entry": true, "flag": true,
	"process": true, "processes": true, "argument": true, "arguments": true,
	"config": true, "configuration": true, "instance": true, "package": true,
	"task": true, "scheduled": true, "server": true, "mcp": true, "agent": true,
}

// entityEmptyNames are what is left once nothing but a description remains: "the
// problem", "the workload", "the setup". A relationship between descriptions is
// noise, and the sweep pays for it by counting it as a source.
var entityEmptyNames = map[string]bool{
	"setup": true, "problem": true, "issue": true, "app": true, "thing": true,
	"workload": true, "system": true, "configuration": true, "config": true,
	"environment": true, "error": true, "server": true, "mcp": true, "agent": true,
	"process": true, "task": true, "integration": true,
	"the": true, "a": true, "an": true,
	// Stripping the trailing noun of a description leaves an adjective behind.
	// "known-working setup" became "known-working" and "the self-hosted instance
	// on a contabo vps" became "self-hosted": still not a name, so not an entity.
	"self-hosted": true, "known-working": true, "functional": true,
	"scheduled": true, "separate": true, "duplicate": true, "seven": true,
	"no longer": true, "relationships": true,
}

// entityLeadingNoise are prefixes that say how something is run, not what it is.
var entityLeadingNoise = []string{
	"npx with ", "npx ", "the ", "a ", "an ", "working ",
	"separate ", "duplicate ", "functional ", "known-working ", "seven ",
}

// entityClauseWords mark a sentence rather than a name. The dry run on
// /projects/infra showed what the trailing-word rule alone leaves behind:
// "base path of the store using the environment variable …", "configuration
// known to work", "backup of the previous tailscale serve". An entity name has no
// clause in it, so these mean the whole name was a description.
var entityClauseWords = map[string]bool{
	"of": true, "to": true, "for": true, "in": true, "on": true, "with": true,
	"using": true, "each": true, "by": true, "at": true, "and": true, "as": true,
}

// NormalizeEntity returns the canonical spelling of an entity name and whether it
// identifies anything at all. A false second result means the name is a
// description ("the problem", "the setup") and an edge carrying it should not be
// stored: it would count as a source for every cluster question it shares a
// target with.
//
// It is deterministic on purpose, so the same rule can be applied once at write
// time, retroactively by `cortexiq canonicalize relationships`, and shown as a
// dry run before it touches anything.
func NormalizeEntity(name string) (string, bool) {
	s := strings.ToLower(strings.Join(strings.Fields(name), " "))
	s = strings.Trim(s, " \t.,;:!?")
	if s == "" {
		return "", false
	}

	for {
		before := s
		for _, prefix := range entityLeadingNoise {
			if strings.HasPrefix(s, prefix) {
				s = strings.TrimSpace(strings.TrimPrefix(s, prefix))
				break
			}
		}
		// "claude desktop on windows" and "hindsight mcp setup in claude desktop"
		// name the same thing as their head; the tail says where it sits.
		for _, cut := range []string{" in ", " on "} {
			if i := strings.LastIndex(s, cut); i > 0 {
				s = s[:i]
			}
		}
		if i := strings.LastIndex(s, " "); i > 0 && entityTrimWords[s[i+1:]] {
			s = s[:i]
		}
		// "hindsight cloud (vectorize.io)" names the same product as "hindsight cloud".
		if strings.HasSuffix(s, ")") {
			if i := strings.LastIndex(s, " ("); i > 0 {
				s = s[:i]
			}
		}
		s = strings.Trim(strings.TrimSpace(s), ".,;:!?")
		// "Hindsight's MCP server" owns the same name as "Hindsight".
		s = strings.TrimSuffix(strings.TrimSpace(s), "'s")
		if s == before {
			break
		}
	}

	if s == "" || entityEmptyNames[s] {
		return "", false
	}
	if isEntityDescription(s) {
		return "", false
	}
	return s, true
}

// isEntityDescription reports whether what is left is a clause or a gerund phrase
// instead of a name. The dry run on production produced both: "backup of the
// previous tailscale serve" (a clause) and "wrapping mcp-remote" (a gerund left
// after cutting the trailing "in conhost.exe --headless").
func isEntityDescription(s string) bool {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return true
	}
	for _, f := range fields[1:] {
		if entityClauseWords[f] {
			return true
		}
	}
	if first := fields[0]; len(first) > 5 && strings.HasSuffix(first, "ing") {
		return true
	}
	return false
}

// EntityRename is one name that folded onto another, with how many rows carried it.
type EntityRename struct {
	Old  string `json:"old"`
	New  string `json:"new"`
	Rows int    `json:"rows"`
}

// EntityCanonicalReport is what a canonicalisation pass did, or would do.
type EntityCanonicalReport struct {
	DryRun      bool           `json:"dry_run"`
	Entities    int            `json:"entities"`
	Changed     int            `json:"changed"`
	Dropped     int            `json:"dropped"`
	RowsMoved   int            `json:"rows_moved"`
	RowsDropped int            `json:"rows_dropped"`
	Duplicates  int            `json:"duplicates_removed"`
	Pairs       []EntityRename `json:"pairs"`
	Removed     []EntityRename `json:"removed"`
}

// CanonicalizeEntities folds the entity names of one or more namespaces onto their
// canonical spelling and drops the relationships whose entity is only a
// description. Existing data is the reason this exists: the extractor may only
// quote names verbatim, so 296 entities grew where forty things live.
//
// With dryRun it only reports the mapping, which is the point of the command: the
// rule is a heuristic, and a heuristic that rewrites evidence should be looked at
// before it runs.
func (b *Brain) CanonicalizeEntities(ctx context.Context, namespaceIDs []int64, dryRun bool) (*EntityCanonicalReport, error) {
	if err := requireWrite(ctx); err != nil && !dryRun {
		return nil, err
	}

	report := &EntityCanonicalReport{DryRun: dryRun, Pairs: []EntityRename{}, Removed: []EntityRename{}}

	for _, nsID := range namespaceIDs {
		rows, err := b.pool.Query(ctx,
			`SELECT name FROM (
			   SELECT DISTINCT lower(btrim(from_entity)) AS name FROM relationships
			    WHERE namespace_id = $1 AND deleted_at IS NULL
			   UNION
			   SELECT DISTINCT lower(btrim(to_entity)) FROM relationships
			    WHERE namespace_id = $1 AND deleted_at IS NULL
			 ) e ORDER BY name`, nsID)
		if err != nil {
			return nil, err
		}
		var names []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				return nil, err
			}
			names = append(names, name)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}

		for _, name := range names {
			report.Entities++
			canonical, keep := NormalizeEntity(name)
			if !keep {
				n, err := b.entityRows(ctx, nsID, name, dryRun, "", true)
				if err != nil {
					return nil, err
				}
				report.Dropped++
				report.RowsDropped += n
				report.Removed = append(report.Removed, EntityRename{Old: name, New: "", Rows: n})
				continue
			}
			if canonical == name {
				continue
			}
			n, err := b.entityRows(ctx, nsID, name, dryRun, canonical, false)
			if err != nil {
				return nil, err
			}
			report.Changed++
			report.RowsMoved += n
			report.Pairs = append(report.Pairs, EntityRename{Old: name, New: canonical, Rows: n})
		}

		if dryRun {
			continue
		}
		dups, err := b.dropDuplicateRelationships(ctx, nsID)
		if err != nil {
			return nil, err
		}
		report.Duplicates += dups
	}

	return report, nil
}

// entityRows rewrites or drops every relationship carrying one entity name and
// reports how many rows it touched. In a dry run it counts without writing.
func (b *Brain) entityRows(ctx context.Context, nsID int64, name string, dryRun bool, canonical string, drop bool) (int, error) {
	if dryRun {
		var n int
		err := b.pool.QueryRow(ctx,
			`SELECT count(*) FROM relationships
			  WHERE namespace_id = $1 AND deleted_at IS NULL
			    AND (lower(btrim(from_entity)) = $2 OR lower(btrim(to_entity)) = $2)`,
			nsID, name).Scan(&n)
		return n, err
	}

	if drop {
		tag, err := b.pool.Exec(ctx,
			`UPDATE relationships SET deleted_at = now()
			  WHERE namespace_id = $1 AND deleted_at IS NULL
			    AND (lower(btrim(from_entity)) = $2 OR lower(btrim(to_entity)) = $2)`,
			nsID, name)
		if err != nil {
			return 0, err
		}
		return int(tag.RowsAffected()), nil
	}

	moved := 0
	for _, column := range []string{"from_entity", "to_entity"} {
		tag, err := b.pool.Exec(ctx,
			`UPDATE relationships SET `+column+` = $3
			  WHERE namespace_id = $1 AND deleted_at IS NULL
			    AND lower(btrim(`+column+`)) = $2`,
			nsID, name, canonical)
		if err != nil {
			return moved, err
		}
		moved += int(tag.RowsAffected())
	}
	return moved, nil
}

// dropDuplicateRelationships removes the rows that became identical after folding
// names, keeping the most confident one — the same rule storeRelationship applies
// when it recognises a repeat.
func (b *Brain) dropDuplicateRelationships(ctx context.Context, nsID int64) (int, error) {
	tag, err := b.pool.Exec(ctx,
		`WITH ranked AS (
		   SELECT id, row_number() OVER (
		     PARTITION BY lower(btrim(from_entity)), lower(btrim(to_entity)),
		                  COALESCE(canonical_type, lower(btrim(relation_type)))
		     ORDER BY confidence DESC, id) AS rn
		   FROM relationships
		    WHERE namespace_id = $1 AND deleted_at IS NULL
		 )
		 UPDATE relationships r SET deleted_at = now()
		 FROM ranked WHERE r.id = ranked.id AND ranked.rn > 1`, nsID)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
