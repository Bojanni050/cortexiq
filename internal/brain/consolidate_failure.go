// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
)

// failureOriginRef names the failures a pattern fact was read from, e.g.
// "failure:7,9,12".
func failureOriginRef(failures []models.Failure) string {
	ids := make([]string, 0, len(failures))
	for _, f := range failures {
		ids = append(ids, strconv.FormatInt(f.ID, 10))
	}
	return "failure:" + strings.Join(ids, ",")
}

func (b *Brain) consolidateFailurePatterns(ctx context.Context, nsID int64, cp *models.ConsolidationProgress) (repeats, patterns, refused, llmCalls int, errs []string) {
	rows, err := b.pool.Query(ctx,
		`SELECT id, namespace_id, goal_id, content, reason, lesson, created_at, deleted_at
		 FROM failures WHERE namespace_id = $1 AND deleted_at IS NULL AND id > $2
		 ORDER BY id LIMIT 50`,
		nsID, cp.LastFailureID,
	)
	if err != nil {
		errs = append(errs, fmt.Sprintf("fetch failures: %v", err))
		return
	}
	defer rows.Close()

	var failures []models.Failure
	var maxFailureID int64
	for rows.Next() {
		var f models.Failure
		if err := rows.Scan(&f.ID, &f.NamespaceID, &f.GoalID, &f.Content, &f.Reason, &f.Lesson, &f.CreatedAt, &f.DeletedAt); err != nil {
			errs = append(errs, fmt.Sprintf("scan failure: %v", err))
			continue
		}
		failures = append(failures, f)
		if f.ID > maxFailureID {
			maxFailureID = f.ID
		}
	}
	if err := rows.Err(); err != nil {
		errs = append(errs, fmt.Sprintf("failure rows: %v", err))
		return
	}

	if len(failures) == 0 {
		return
	}

	epSQL, epArgs, err := b.queries.FetchEpisodes(nsID, cp.LastFailureEpisodeID, 30)
	if err != nil {
		errs = append(errs, fmt.Sprintf("build fetch episodes for failures: %v", err))
		return
	}

	epRows, err := b.pool.Query(ctx, epSQL, epArgs...)
	if err != nil {
		errs = append(errs, fmt.Sprintf("fetch episodes for failures: %v", err))
		return
	}
	defer epRows.Close()

	var episodeTexts []string
	var episodeIDs []int64
	var maxEpisodeID int64
	for epRows.Next() {
		var e models.Episode
		if err := epRows.Scan(&e.ID, &e.NamespaceID, &e.Content, &e.Embedding, &e.EmbeddingModel, &e.OccurredAt, &e.CreatedAt); err != nil {
			errs = append(errs, fmt.Sprintf("scan episode for failures: %v", err))
			continue
		}
		episodeTexts = append(episodeTexts, e.Content)
		episodeIDs = append(episodeIDs, e.ID)
		if e.ID > maxEpisodeID {
			maxEpisodeID = e.ID
		}
	}
	if err := epRows.Err(); err != nil {
		errs = append(errs, fmt.Sprintf("episode rows for failures: %v", err))
		return
	}

	if len(episodeTexts) == 0 {
		if maxFailureID > cp.LastFailureID {
			cp.LastFailureID = maxFailureID
		}
		return
	}

	llmCalls++
	// A pattern is read out of the whole batch, so the batch is its source: the
	// failures it saw and the episodes it saw. Coarse, but true.
	failureRef := failureOriginRef(failures)
	results, err := b.reasoner.ReasonFailurePatterns(ctx, failures, episodeTexts)
	if err != nil {
		errs = append(errs, fmt.Sprintf("reason failure patterns: %v", err))
		return
	}

	for _, r := range results {
		switch r.Type {
		case "repetition":
			content := fmt.Sprintf("REPEAT FAILURE [failure #%d]: %s", r.FailureID, r.Evidence)
			_, err := b.pool.Exec(ctx,
				`INSERT INTO episodes (namespace_id, content, embedding, embedding_model, occurred_at)
				 VALUES ($1, $2, NULL, '', $3)`,
				nsID, content, time.Now().UTC(),
			)
			if err != nil {
				errs = append(errs, fmt.Sprintf("insert repeat failure episode: %v", err))
				continue
			}
			repeats++

		case "pattern":
			if r.PatternFact == "" {
				continue
			}
			vec, embErr := b.embedder.Embed(ctx, r.PatternFact)
			if embErr != nil {
				errs = append(errs, fmt.Sprintf("embed failure pattern fact: %v", embErr))
				continue
			}
			validFrom := time.Now().UTC()
			if _, err := b.insertFact(ctx, b.pool, factInsert{
				Origin:         originFailure,
				NamespaceID:    nsID,
				Content:        r.PatternFact,
				Embedding:      vec,
				EmbeddingModel: b.embedder.Model(),
				Confidence:     r.Confidence,
				ValidFrom:      &validFrom,
				Importance:     DefaultImportance,

				SourceEpisodeIDs: episodeIDs,
				OriginRef:        failureRef,
			}); err != nil {
				if errors.Is(err, ErrRejectedValue) {
					// A pattern the user rejected earlier. Counted, not an error.
					refused++
					continue
				}
				errs = append(errs, fmt.Sprintf("insert failure pattern fact: %v", err))
				continue
			}
			patterns++
		}
	}

	if maxFailureID > cp.LastFailureID {
		cp.LastFailureID = maxFailureID
	}
	if maxEpisodeID > cp.LastFailureEpisodeID {
		cp.LastFailureEpisodeID = maxEpisodeID
	}

	return
}
