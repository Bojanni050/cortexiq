// Modifications copyright (c) 2026 Bojan van der Heide
// Package models defines domain structs for pgx scanning.
// Every field tag matches the PostgreSQL column name exactly.
package models

import (
	"time"

	"github.com/pgvector/pgvector-go"
)

// Namespace owns memory.
type Namespace struct {
	ID          int64     `db:"id" json:"id"`
	Slug        string    `db:"slug" json:"slug"`
	Name        string    `db:"name" json:"name"`
	Description string    `db:"description" json:"description"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// Episode is an immutable, append-only raw event.
type Episode struct {
	ID             int64           `db:"id"`
	NamespaceID    int64           `db:"namespace_id"`
	Content        string          `db:"content"`
	Embedding      pgvector.Vector `db:"embedding"`
	EmbeddingModel string          `db:"embedding_model"`
	OccurredAt     time.Time       `db:"occurred_at"`
	CreatedAt      time.Time       `db:"created_at"`
	DeletedAt      *time.Time      `db:"deleted_at"`
}

// Fact is a belief derived from episodes.
type Fact struct {
	ID             int64           `db:"id"`
	NamespaceID    int64           `db:"namespace_id"`
	Content        string          `db:"content"`
	Embedding      pgvector.Vector `db:"embedding"`
	EmbeddingModel string          `db:"embedding_model"`
	Confidence     float32         `db:"confidence"`
	Entity         *string         `db:"entity"`
	Property       *string         `db:"property"`
	Value          *string         `db:"value"`
	ValidFrom      *time.Time      `db:"valid_from"`
	ValidUntil     *time.Time      `db:"valid_until"`
	CreatedAt      time.Time       `db:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at"`
	DeletedAt      *time.Time      `db:"deleted_at"`
}

// Contradiction records a conflict between two facts about the same entity and property.
type Contradiction struct {
	ID          int64      `db:"id" json:"id"`
	NamespaceID int64      `db:"namespace_id" json:"namespace_id"`
	OldFactID   int64      `db:"old_fact_id" json:"old_fact_id"`
	NewFactID   int64      `db:"new_fact_id" json:"new_fact_id"`
	Entity      string     `db:"entity" json:"entity"`
	Property    string     `db:"property" json:"property"`
	OldValue    string     `db:"old_value" json:"old_value"`
	NewValue    string     `db:"new_value" json:"new_value"`
	Confidence  float32    `db:"confidence" json:"confidence"`
	Method      string     `db:"method" json:"method"`
	Resolved    bool       `db:"resolved" json:"resolved"`
	Resolution  *string    `db:"resolution" json:"resolution"`
	ResolvedAt  *time.Time `db:"resolved_at" json:"resolved_at"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
}

// FactSource links a fact to the episodes that support it.
type FactSource struct {
	FactID    int64 `db:"fact_id"`
	EpisodeID int64 `db:"episode_id"`
}

// Relationship is an extracted entity edge.
type Relationship struct {
	ID           int64  `db:"id"`
	NamespaceID  int64  `db:"namespace_id"`
	FromEntity   string `db:"from_entity"`
	RelationType string `db:"relation_type"`
	// CanonicalType is the relation type mapped onto the fixed vocabulary; nil
	// when there is no confident mapping (RelationType is always the original).
	CanonicalType *string    `db:"canonical_type"`
	ToEntity      string     `db:"to_entity"`
	Confidence    float32    `db:"confidence"`
	SourceFactID  *int64     `db:"source_fact_id"`
	CreatedAt     time.Time  `db:"created_at"`
	DeletedAt     *time.Time `db:"deleted_at"`
}

// Pattern is an abstraction over facts and relationships.
type Pattern struct {
	ID             int64      `db:"id"`
	NamespaceID    int64      `db:"namespace_id"`
	Content        string     `db:"content"`
	Confidence     float32    `db:"confidence"`
	SourceFactIDs  []int64    `db:"source_fact_ids"`
	SourceRelIDs   []int64    `db:"source_rel_ids"`
	CoherenceScore float32    `db:"coherence_score"`
	CreatedAt      time.Time  `db:"created_at"`
	UpdatedAt      time.Time  `db:"updated_at"`
	DeletedAt      *time.Time `db:"deleted_at"`
}

// Context is the active working state for a namespace.
type Context struct {
	NamespaceID int64     `db:"namespace_id"`
	Focus       string    `db:"focus"`
	ExpiresAt   time.Time `db:"expires_at"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

// CausalLink records a cause-effect relationship between two facts.
type CausalLink struct {
	ID           int64      `db:"id" json:"id"`
	NamespaceID  int64      `db:"namespace_id" json:"namespace_id"`
	CauseFactID  int64      `db:"cause_fact_id" json:"cause_fact_id"`
	EffectFactID int64      `db:"effect_fact_id" json:"effect_fact_id"`
	Confidence   float32    `db:"confidence" json:"confidence"`
	Method       string     `db:"method" json:"method"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	DeletedAt    *time.Time `db:"deleted_at" json:"deleted_at,omitempty"`
}

// Hypothesis is a belief held with uncertainty plus a plan to verify it.
type Hypothesis struct {
	ID               int64      `db:"id" json:"id"`
	NamespaceID      int64      `db:"namespace_id" json:"namespace_id"`
	Content          string     `db:"content" json:"content"`
	Confidence       float32    `db:"confidence" json:"confidence"`
	Status           string     `db:"status" json:"status"`
	VerificationPlan string     `db:"verification_plan" json:"verification_plan"`
	Method           string     `db:"method" json:"method"`
	ConfirmedFactID  *int64     `db:"confirmed_fact_id" json:"confirmed_fact_id"`
	RejectionReason  *string    `db:"rejection_reason" json:"rejection_reason"`
	SourceFactIDs    []int64    `db:"source_fact_ids" json:"source_fact_ids"`
	TestedAt         *time.Time `db:"tested_at" json:"tested_at"`
	ConfirmedAt      *time.Time `db:"confirmed_at" json:"confirmed_at"`
	RejectedAt       *time.Time `db:"rejected_at" json:"rejected_at"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at" json:"updated_at"`
	DeletedAt        *time.Time `db:"deleted_at" json:"deleted_at,omitempty"`
}

// Goal is an intended outcome that persists across sessions.
type Goal struct {
	ID          int64      `db:"id" json:"id"`
	NamespaceID int64      `db:"namespace_id" json:"namespace_id"`
	ParentID    *int64     `db:"parent_id" json:"parent_id"`
	Content     string     `db:"content" json:"content"`
	Status      string     `db:"status" json:"status"`
	Priority    int        `db:"priority" json:"priority"`
	Notes       string     `db:"notes" json:"notes"`
	CompletedAt *time.Time `db:"completed_at" json:"completed_at"`
	AbandonedAt *time.Time `db:"abandoned_at" json:"abandoned_at"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updated_at"`
	DeletedAt   *time.Time `db:"deleted_at" json:"deleted_at,omitempty"`
}

// Failure records what didn't work, why, and what to do instead.
type Failure struct {
	ID          int64      `db:"id" json:"id"`
	NamespaceID int64      `db:"namespace_id" json:"namespace_id"`
	GoalID      *int64     `db:"goal_id" json:"goal_id"`
	Content     string     `db:"content" json:"content"`
	Reason      string     `db:"reason" json:"reason"`
	Lesson      string     `db:"lesson" json:"lesson"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	DeletedAt   *time.Time `db:"deleted_at" json:"deleted_at,omitempty"`
}

// ConsolidationProgress tracks per-stage checkpoint per namespace.
type ConsolidationProgress struct {
	NamespaceID            int64      `db:"namespace_id"`
	LastEpisodeID          int64      `db:"last_episode_id"`
	LastFactID             int64      `db:"last_fact_id"`
	LastRelationshipID     int64      `db:"last_relationship_id"`
	LastPatternFactID      int64      `db:"last_pattern_fact_id"`
	LastPatternRelID       int64      `db:"last_pattern_rel_id"`
	LastGoalProgressFactID int64      `db:"last_goal_progress_fact_id"`
	LastFailureID          int64      `db:"last_failure_id"`
	LastFailureEpisodeID   int64      `db:"last_failure_episode_id"`
	LastHypothesisFactID   int64      `db:"last_hypothesis_fact_id"`
	LastCausalFactID       int64      `db:"last_causal_fact_id"`
	LastDecayRun           *time.Time `db:"last_decay_run"`
	LastRun                *time.Time `db:"last_run"`
	UpdatedAt              time.Time  `db:"updated_at"`
}

// Setting is a key-value store for operational state.
type Setting struct {
	Key       string    `db:"key"`
	Value     string    `db:"value"`
	UpdatedAt time.Time `db:"updated_at"`
}

// EmbeddingCache stores computed embeddings by text hash and model.
type EmbeddingCache struct {
	TextHash  string          `db:"text_hash"`
	Model     string          `db:"model"`
	Text      string          `db:"text"`
	Embedding pgvector.Vector `db:"embedding"`
	CreatedAt time.Time       `db:"created_at"`
}

// RecallResult carries a row from recall queries.
type RecallResult struct {
	ID             int64           `db:"id"`
	NamespaceID    int64           `db:"namespace_id"`
	Content        string          `db:"content"`
	Embedding      pgvector.Vector `db:"embedding"`
	EmbeddingModel string          `db:"embedding_model"`
	OccurredAt     time.Time       `db:"occurred_at"`
	CreatedAt      time.Time       `db:"created_at"`
	Score          float32         `db:"score"`
}

// ConsolidationRun records the outcome of one consolidation run for a namespace.
type ConsolidationRun struct {
	ID                      int64     `db:"id" json:"id"`
	NamespaceID             int64     `db:"namespace_id" json:"namespace_id"`
	NamespaceSlug           string    `db:"namespace_slug" json:"namespace_slug"`
	StartedAt               time.Time `db:"started_at" json:"started_at"`
	DurationMS              int64     `db:"duration_ms" json:"duration_ms"`
	EpisodesRead            int       `db:"episodes_read" json:"episodes_read"`
	EpisodesSkipped         int       `db:"episodes_skipped" json:"episodes_skipped"`
	FactsCreated            int       `db:"facts_created" json:"facts_created"`
	FactsDeduplicated       int       `db:"facts_deduplicated" json:"facts_deduplicated"`
	RelationshipsFound      int       `db:"relationships_found" json:"relationships_found"`
	CausalLinksFound        int       `db:"causal_links_found" json:"causal_links_found"`
	PatternsFound           int       `db:"patterns_found" json:"patterns_found"`
	ContradictionsFound     int       `db:"contradictions_found" json:"contradictions_found"`
	GoalsAnnotated          int       `db:"goals_annotated" json:"goals_annotated"`
	GoalsSuggestedComplete  int       `db:"goals_suggested_complete" json:"goals_suggested_complete"`
	FailureRepeatsDetected  int       `db:"failure_repeats_detected" json:"failure_repeats_detected"`
	FailurePatternsFound    int       `db:"failure_patterns_found" json:"failure_patterns_found"`
	HypothesesAutoConfirmed int       `db:"hypotheses_auto_confirmed" json:"hypotheses_auto_confirmed"`
	HypothesesAutoRejected  int       `db:"hypotheses_auto_rejected" json:"hypotheses_auto_rejected"`
	HypothesesUpdated       int       `db:"hypotheses_updated" json:"hypotheses_updated"`
	FactsDecayed            int       `db:"facts_decayed" json:"facts_decayed"`
	FactsExpired            int       `db:"facts_expired" json:"facts_expired"`
	DedupNearMisses         int       `db:"dedup_near_misses" json:"dedup_near_misses"`
	LLMCalls                int       `db:"llm_calls" json:"llm_calls"`
	LLMCallsLimited         bool      `db:"llm_calls_limited" json:"llm_calls_limited"`
	Errors                  int       `db:"errors" json:"errors"`
	ErrorSample             string    `db:"error_sample" json:"error_sample"`
	CreatedAt               time.Time `db:"created_at" json:"created_at"`
}

// MentalModel is a curated, high-quality summary about a specific topic,
// generated by a reflect call and stored for fast retrieval. Unlike facts
// (extracted from episodes), a mental model is a persistent artifact that is
// refreshed when new memories arrive.
type MentalModel struct {
	ID              int64           `db:"id" json:"id"`
	NamespaceID     int64           `db:"namespace_id" json:"namespace_id"`
	Name            string          `db:"name" json:"name"`
	SourceQuery     string          `db:"source_query" json:"source_query"`
	Content         string          `db:"content" json:"content"`
	Embedding       pgvector.Vector `db:"embedding" json:"-"`
	EmbeddingModel  string          `db:"embedding_model" json:"embedding_model,omitempty"`
	Tags            []string        `db:"tags" json:"tags"`
	MaxTokens       int             `db:"max_tokens" json:"max_tokens"`
	LastRefreshedAt *time.Time      `db:"last_refreshed_at" json:"last_refreshed_at,omitempty"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at" json:"updated_at"`
	DeletedAt       *time.Time      `db:"deleted_at" json:"deleted_at,omitempty"`
}
