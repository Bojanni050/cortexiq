// Modifications copyright (c) 2026 Bojan van der Heide
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Bojanni050/cortexiq/internal/auth"
	"github.com/Bojanni050/cortexiq/internal/brain"
	"github.com/Bojanni050/cortexiq/internal/config"
	"github.com/Bojanni050/cortexiq/internal/db"
	"github.com/Bojanni050/cortexiq/internal/embedder"
	"github.com/Bojanni050/cortexiq/internal/observability"
	"github.com/Bojanni050/cortexiq/internal/queries"
	"github.com/Bojanni050/cortexiq/internal/reasoner"
	"github.com/Bojanni050/cortexiq/internal/reranker"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Context holds all initialized services.
type Context struct {
	Config *config.Config
	Brain  *brain.Brain
	Pool   *pgxpool.Pool
	Logger *slog.Logger
	Auth   *auth.Set

	// embedder is the raw (uncached) embedder, kept for the start-up warm-up.
	embedder embedder.Embedder
	// warmOnce makes sure the warm-up runs at most once per process, however
	// many servers call it.
	warmOnce sync.Once
}

// MustNew panics on bootstrap failure.
func MustNew(ctx context.Context) *Context {
	bc, err := New(ctx)
	if err != nil {
		panic(fmt.Sprintf("bootstrap failed: %v", err))
	}
	return bc
}

// New initializes all services: database, embedder, reasoner, queries, brain.
func New(ctx context.Context) (*Context, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	logger := buildLogger(cfg)

	tokenSet, err := auth.ParseSet(cfg.APITokens, cfg.APIToken)
	if err != nil {
		return nil, fmt.Errorf("api tokens: %w", err)
	}
	if names := tokenSet.LegacyNamespaces(); len(names) > 0 {
		logger.Warn("CORTEXIQ_API_TOKENS still sets a namespace allowlist, which is no longer enforced; these tokens can reach every namespace - remove the \"namespaces\" field",
			"tokens", names)
	}

	pool, err := db.Open(ctx, cfg.StoreDSN, cfg.EmbeddingModel, cfg.VectorDim)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	emb, err := buildEmbedder(cfg)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("build embedder: %w", err)
	}

	// Wrap embedder with pgx-backed cache
	cachedEmb := embedder.NewCached(emb, pool)

	reas, err := buildReasoner(cfg)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("build reasoner: %w", err)
	}

	if cfg.ModelPrices != "" {
		modelPrices, err := observability.ParsePrices(cfg.ModelPrices)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("model prices: %w", err)
		}
		observability.SetModelPrices(modelPrices)
	}

	q, err := queries.New()
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("load queries: %w", err)
	}

	window, err := time.ParseDuration(cfg.ConsolidationWindow)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("parse consolidation window: %w", err)
	}

	rerank := reranker.NewHTTP(cfg.RerankerBaseURL, cfg.RerankerAPIKey, cfg.RerankerModel)
	br, err := brain.New(pool, cachedEmb, reas, q, brain.Config{
		BatchSize:                      cfg.ConsolidationBatchSize,
		SimilarityThreshold:            cfg.ConsolidationSimilarityThreshold,
		DedupThreshold:                 cfg.ConsolidationDedupThreshold,
		DedupReviewThreshold:           cfg.ConsolidationDedupReviewThreshold,
		DedupCandidates:                cfg.ConsolidationDedupCandidates,
		CandidateCap:                   cfg.ConsolidationCandidateCap,
		LexicalFloor:                   cfg.ConsolidationLexicalFloor,
		Window:                         window,
		DecayFactor:                    cfg.DecayFactor,
		ExpiryThreshold:                cfg.ExpiryThreshold,
		HypothesisAutoConfirmThreshold: cfg.HypothesisAutoConfirmThreshold,
		HypothesisAutoRejectThreshold:  cfg.HypothesisAutoRejectThreshold,
		RelationshipBatchSize:          cfg.RelationshipBatchSize,
		MaxLLMCallsPerRun:              cfg.MaxLLMCallsPerRun,
		HybridRecall:                   cfg.HybridRecall,
		GraphRecall:                    cfg.GraphRecall,
		RouteRecall:                    cfg.RouteRecall,
		TemporalRecall:                 cfg.TemporalRecall,
		TemporalArm:                    cfg.TemporalArm,
		MinRecallSimilarity:            cfg.RecallMinSimilarity,
		RecencyWeight:                  cfg.RecallRecencyWeight,
		RecencyHalfLife:                cfg.RecallRecencyHalfLife,
		ImportanceWeight:               cfg.RecallImportanceWeight,
		MMRLambda:                      cfg.RecallMMRLambda,
		Reranker:                       rerank,
		RerankCandidates:               cfg.RerankCandidates,
		MinEpisodeLength:               cfg.ExtractionMinChars,
		SocialFilter:                   cfg.SocialFilter,
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("build brain: %w", err)
	}

	return &Context{
		Config:   cfg,
		Brain:    br,
		Pool:     pool,
		Logger:   logger,
		Auth:     tokenSet,
		embedder: emb,
	}, nil
}

// embedWarmupText is embedded once at start-up; only the call matters, not the
// text, so a fixed short string is enough.
const embedWarmupText = "cortexiq embedder warm-up"

// embedWarmupTimeout bounds that call, so an unreachable backend cannot keep a
// goroutine alive forever.
const embedWarmupTimeout = 30 * time.Second

// WarmEmbedder pulls the embedding model into memory at start-up instead of
// during the first recall: loading it costs ~1.8s against ~5ms once warm, and a
// background prefetch (as agent plugins do) otherwise pays that on the user's
// first question after a long idle period.
//
// It runs in the background, at most once per process, and never fails the
// caller: recall reports backend problems itself, with a better context.
func (c *Context) WarmEmbedder(ctx context.Context) {
	if c == nil || c.embedder == nil {
		return
	}
	c.warmOnce.Do(func() {
		go func() {
			warmCtx, cancel := context.WithTimeout(ctx, embedWarmupTimeout)
			defer cancel()
			start := time.Now()
			if _, err := c.embedder.Embed(warmCtx, embedWarmupText); err != nil {
				c.Logger.Warn("embedder warm-up failed; the first recall will pay for it", "err", err)
				return
			}
			c.Logger.Info("embedder warm", "took", time.Since(start).String())
		}()
	})
}

// Close releases all resources.
func (c *Context) Close() error {
	var errs []string
	if c.Brain != nil {
		c.Brain.Close()
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func loadConfig() (*config.Config, error) {
	filename := os.Getenv(config.ConfigEnvVar)
	if filename == "" {
		filename = ".env"
	}
	return config.NewFromFile(filename)
}

func buildLogger(cfg *config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{}

	switch cfg.LogLevel {
	case "debug":
		opts.Level = slog.LevelDebug
	case "info":
		opts.Level = slog.LevelInfo
	case "warn":
		opts.Level = slog.LevelWarn
	case "error":
		opts.Level = slog.LevelError
	default:
		opts.Level = slog.LevelInfo
	}

	if cfg.LogFormat == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

func buildEmbedder(cfg *config.Config) (embedder.Embedder, error) {
	baseURL, apiKey := cfg.OpenAIBaseURL, cfg.OpenAIAPIKey
	if cfg.EmbeddingBaseURL != "" {
		baseURL = cfg.EmbeddingBaseURL
	}
	if cfg.EmbeddingAPIKey != "" {
		apiKey = cfg.EmbeddingAPIKey
	}
	return embedder.NewOpenAI(
		baseURL,
		apiKey,
		cfg.EmbeddingModel,
		cfg.VectorDim,
	)
}

func buildReasoner(cfg *config.Config) (reasoner.Reasoner, error) {
	return reasoner.NewOpenAI(
		cfg.OpenAIBaseURL,
		cfg.OpenAIAPIKey,
		cfg.ReasonerModel,
		cfg.ReasonerMaxTokens,
		cfg.ReasonerJSONMode,
	)
}
