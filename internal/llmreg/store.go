// Modifications copyright (c) 2026 Bojan van der Heide
package llmreg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Purposes are the four places a model can be configured. They match the
// llm_bindings CHECK constraint.
const (
	PurposeReasoner  = "reasoner"
	PurposeTriage    = "triage"
	PurposeEmbedding = "embedding"
	PurposeReranker  = "reranker"
)

var (
	ErrProviderNotFound = errors.New("llmreg: provider not found")
	ErrPurposeUnknown   = errors.New("llmreg: unknown purpose")
)

// Binding records which model a purpose uses, when it was chosen in the
// dashboard rather than in the environment.
type Binding struct {
	Purpose  string `json:"purpose"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Updated  string `json:"updated_at"`
}

// Store is the database side of the registry: providers and their bindings.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func validPurpose(purpose string) bool {
	switch purpose {
	case PurposeReasoner, PurposeTriage, PurposeEmbedding, PurposeReranker:
		return true
	}
	return false
}

// Providers lists every configured provider. The API key is left in the struct
// for the caller that needs to make a request; handlers must return Provider.Masked().
func (s *Store) Providers(ctx context.Context) ([]Provider, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT slug, name, base_url, api_key, created_at, updated_at
		 FROM llm_providers ORDER BY slug`)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer rows.Close()

	out := []Provider{}
	for rows.Next() {
		var p Provider
		if err := rows.Scan(&p.Slug, &p.Name, &p.BaseURL, &p.APIKey, &p.Created, &p.Updated); err != nil {
			return nil, fmt.Errorf("list providers: scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Provider loads one provider by slug, key included.
func (s *Store) Provider(ctx context.Context, slug string) (Provider, error) {
	var p Provider
	err := s.pool.QueryRow(ctx,
		`SELECT slug, name, base_url, api_key, created_at, updated_at
		 FROM llm_providers WHERE slug = $1`, slug,
	).Scan(&p.Slug, &p.Name, &p.BaseURL, &p.APIKey, &p.Created, &p.Updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return Provider{}, ErrProviderNotFound
	}
	if err != nil {
		return Provider{}, fmt.Errorf("load provider: %w", err)
	}
	return p, nil
}

// UpsertProvider creates a provider or updates it. An empty APIKey on an update
// means "keep the stored key": the dashboard never receives the key back, so it
// cannot send it again, and treating an empty field as a delete would wipe the
// credential every time someone edits the name.
func (s *Store) UpsertProvider(ctx context.Context, p Provider) (Provider, error) {
	p.Slug = strings.TrimSpace(p.Slug)
	p.Name = strings.TrimSpace(p.Name)
	p.BaseURL = strings.TrimSpace(strings.TrimRight(p.BaseURL, "/"))
	if p.Slug == "" || p.BaseURL == "" {
		return Provider{}, fmt.Errorf("llmreg: slug and base URL are required")
	}
	if p.Name == "" {
		p.Name = p.Slug
	}

	var out Provider
	err := s.pool.QueryRow(ctx,
		`INSERT INTO llm_providers (slug, name, base_url, api_key)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (slug) DO UPDATE SET
		     name = EXCLUDED.name,
		     base_url = EXCLUDED.base_url,
		     api_key = CASE WHEN EXCLUDED.api_key = '' THEN llm_providers.api_key ELSE EXCLUDED.api_key END,
		     updated_at = now()
		 RETURNING slug, name, base_url, api_key, created_at, updated_at`,
		p.Slug, p.Name, p.BaseURL, p.APIKey,
	).Scan(&out.Slug, &out.Name, &out.BaseURL, &out.APIKey, &out.Created, &out.Updated)
	if err != nil {
		return Provider{}, fmt.Errorf("save provider: %w", err)
	}
	return out, nil
}

// Bindings returns the model chosen per purpose. A purpose without a row is
// absent from the map, which means "use the environment".
func (s *Store) Bindings(ctx context.Context) (map[string]Binding, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT purpose, provider_slug, model, updated_at FROM llm_bindings`)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	defer rows.Close()

	out := map[string]Binding{}
	for rows.Next() {
		var b Binding
		var updated time.Time
		if err := rows.Scan(&b.Purpose, &b.Provider, &b.Model, &updated); err != nil {
			return nil, fmt.Errorf("list bindings: scan: %w", err)
		}
		b.Updated = updated.UTC().Format(time.RFC3339)
		out[b.Purpose] = b
	}
	return out, rows.Err()
}

// SetBinding points a purpose at a provider and model. The provider must exist.
func (s *Store) SetBinding(ctx context.Context, purpose, providerSlug, model string) error {
	if !validPurpose(purpose) {
		return ErrPurposeUnknown
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return ErrModelRequired
	}
	if _, err := s.Provider(ctx, providerSlug); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO llm_bindings (purpose, provider_slug, model)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (purpose) DO UPDATE SET
		     provider_slug = EXCLUDED.provider_slug,
		     model = EXCLUDED.model,
		     updated_at = now()`,
		purpose, providerSlug, model,
	)
	if err != nil {
		return fmt.Errorf("save binding: %w", err)
	}
	return nil
}

// ClearBinding returns a purpose to the environment-configured model.
func (s *Store) ClearBinding(ctx context.Context, purpose string) error {
	if !validPurpose(purpose) {
		return ErrPurposeUnknown
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM llm_bindings WHERE purpose = $1`, purpose); err != nil {
		return fmt.Errorf("clear binding: %w", err)
	}
	return nil
}
