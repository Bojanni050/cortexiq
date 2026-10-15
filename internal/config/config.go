// Modifications copyright (c) 2026 Bojan van der Heide
package config

import (
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config keys are read from CORTEXIQ_* environment variables; CORTEXIQCONFIG
// names the .env file to load.
const (
	EnvPrefix    = "CORTEXIQ_"
	ConfigEnvVar = "CORTEXIQCONFIG"
)

type Config struct {
	// Store (PostgreSQL only)
	StoreDSN      string `env:"CORTEXIQ_POSTGRES_DSN,required"`
	VectorDim     int    `env:"CORTEXIQ_VECTOR_DIM,required"`
	MaxResultSize int    `env:"CORTEXIQ_MAX_RESULT_SIZE,required"`

	// OpenAI (embeddings + reasoning)
	OpenAIAPIKey   string `env:"CORTEXIQ_OPENAI_API_KEY,required"`
	OpenAIBaseURL  string `env:"CORTEXIQ_OPENAI_BASE_URL,required"`
	EmbeddingModel string `env:"CORTEXIQ_EMBEDDING_MODEL,required"`
	ReasonerModel  string `env:"CORTEXIQ_REASONER_MODEL,required"`

	// Reasoner request tuning. MaxTokens caps output tokens per call (0 =
	// unlimited); it guards against verbose/reasoning models burning output
	// tokens during JSON extraction. JSONMode requests a native JSON response
	// (response_format=json_object) where the provider/model supports it.
	ReasonerMaxTokens int  `env:"CORTEXIQ_REASONER_MAX_TOKENS" envDefault:"1024"`
	ReasonerJSONMode  bool `env:"CORTEXIQ_REASONER_JSON_MODE" envDefault:"false"`

	// ModelPrices feeds the cost estimate: "model=input:output,..." per million
	// tokens. Empty disables cost accounting; token metrics still work.
	ModelPrices string `env:"CORTEXIQ_MODEL_PRICES" envDefault:""`

	// Optional overrides so embeddings can use a different endpoint (e.g. a local
	// Ollama) than the reasoner. Fall back to the OpenAI settings above when empty.
	// The `envDefault:""` is what keeps them optional: config parsing runs with
	// RequiredIfNoDef, so a field with no default would otherwise be required —
	// which is exactly what made the server refuse to start without them.
	EmbeddingBaseURL string `env:"CORTEXIQ_EMBEDDING_BASE_URL" envDefault:""`
	EmbeddingAPIKey  string `env:"CORTEXIQ_EMBEDDING_API_KEY" envDefault:""`

	// Memory
	ContextTTL time.Duration `env:"CORTEXIQ_CONTEXT_TTL,required"`

	// Server
	// APIToken, when set, is required as a Bearer token on the dashboard API
	// (/api/v1). Empty keeps the API open. `envDefault:""` keeps it optional.
	APIToken string `env:"CORTEXIQ_API_TOKEN" envDefault:""`
	// APITokens is a JSON array of {name, token, scope}. When set it replaces
	// APIToken and lets each caller be scoped: "rw" (default) or "ro".
	APITokens string `env:"CORTEXIQ_API_TOKENS" envDefault:""`
	HTTPAddr  string `env:"CORTEXIQ_HTTP_ADDR,required"`
	LogLevel  string `env:"CORTEXIQ_LOG_LEVEL,required"`
	LogFormat string `env:"CORTEXIQ_LOG_FORMAT,required"`

	// Consolidation
	ConsolidationBatchSize           int     `env:"CORTEXIQ_CONSOLIDATION_BATCH_SIZE" envDefault:"100"`
	ConsolidationSimilarityThreshold float64 `env:"CORTEXIQ_CONSOLIDATION_SIMILARITY_THRESHOLD" envDefault:"0.85"`
	ConsolidationDedupThreshold      float64 `env:"CORTEXIQ_CONSOLIDATION_DEDUP_THRESHOLD" envDefault:"0.85"`
	// From this similarity up to the dedup threshold the LLM decides whether a new
	// fact is a rewording/translation of its nearest neighbour (0 = off).
	ConsolidationDedupReviewThreshold float64 `env:"CORTEXIQ_CONSOLIDATION_DEDUP_REVIEW_THRESHOLD" envDefault:"0.80"`
	ConsolidationWindow               string  `env:"CORTEXIQ_CONSOLIDATION_WINDOW" envDefault:"168h"`
	// HybridRecall adds full-text search to recall (fused with the vector ranking).
	HybridRecall bool `env:"CORTEXIQ_HYBRID_RECALL" envDefault:"true"`
	// RecallMinSimilarity drops vector-only recall results below this cosine
	// similarity (results confirmed by keyword search always stay). 0 = off. The
	// right value depends on the embedding model: measure relevant vs irrelevant
	// queries first (bge-m3 separates around 0.40).
	RecallMinSimilarity float32 `env:"CORTEXIQ_RECALL_MIN_SIMILARITY" envDefault:"0"`
	// RecallRecencyWeight (0..1) lets recall prefer newer results: a very old
	// result keeps 1-weight of its score. 0 = off (default, ranking unchanged).
	RecallRecencyWeight float64 `env:"CORTEXIQ_RECALL_RECENCY_WEIGHT" envDefault:"0"`
	// RecallRecencyHalfLife is the age at which half of the recency effect is gone.
	RecallRecencyHalfLife time.Duration `env:"CORTEXIQ_RECALL_RECENCY_HALFLIFE" envDefault:"168h"`
	// RecallImportanceWeight (0..1) lets recall prefer memories marked important
	// when they were remembered: importance 0 keeps 1-weight of the score,
	// importance 1 keeps all of it. 0 = off (default, ranking unchanged).
	RecallImportanceWeight float64 `env:"CORTEXIQ_RECALL_IMPORTANCE_WEIGHT" envDefault:"0"`
	// RelationshipBatchSize facts are sent per relationship-extraction LLM call.
	// Keep it modest: the reply must fit in CORTEXIQ_REASONER_MAX_TOKENS.
	RelationshipBatchSize int `env:"CORTEXIQ_RELATIONSHIP_BATCH_SIZE" envDefault:"5"`
	// MaxLLMCallsPerRun hard-limits reasoner API calls in one namespace
	// consolidation. 0 disables the limit.
	MaxLLMCallsPerRun              int     `env:"CORTEXIQ_MAX_LLM_CALLS_PER_RUN" envDefault:"0"`
	DecayFactor                    float64 `env:"CORTEXIQ_DECAY_FACTOR" envDefault:"0.95"`
	ExpiryThreshold                float32 `env:"CORTEXIQ_EXPIRY_THRESHOLD" envDefault:"0.1"`
	HypothesisAutoConfirmThreshold float32 `env:"CORTEXIQ_HYPOTHESIS_AUTO_CONFIRM_THRESHOLD" envDefault:"0.9"`
	HypothesisAutoRejectThreshold  float32 `env:"CORTEXIQ_HYPOTHESIS_AUTO_REJECT_THRESHOLD" envDefault:"0.9"`
}

// NewFromFile loads filename into the process environment (if it exists) and
// parses the configuration from it.
func NewFromFile(filename string) (*Config, error) {
	if _, err := os.Stat(filename); err == nil {
		if err := godotenv.Load(filename); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	cfg := &Config{}
	opts := env.Options{
		RequiredIfNoDef: true,
	}
	if err := env.ParseWithOptions(cfg, opts); err != nil {
		return nil, err
	}
	return cfg, nil
}
