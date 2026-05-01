package logstruct

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Tragidra/logstruct/internal/config"
)

// Config is the public configuration for the logstruct library, you can pass it to New();
// or load it from disk with NewFromYAML(), like /internal/config
//
// All fields have sensible defaults — only Sources or Inline. Enabled is
// strictly required (you need at least one input)
type Config struct {
	// Sources to import, may be empty if Inline mode is the only input.
	Sources []SourceConfig `yaml:"sources"`

	// AI provider configuration.
	AI AIConfig `yaml:"ai"`

	// Where to persist clusters and analyses, defaults to in-memory.
	Storage StorageConfig `yaml:"storage"`

	// Inline mode — when enabled, ll.Report() pushes events into the
	// pipeline so user code errors are clustered with file-sourced logs.
	Inline InlineConfig `yaml:"inline"`

	// Clustering tuning, sensible defaults if zero.
	Cluster ClusterConfig `yaml:"cluster"`

	// Logger. If nil, a default slog handler at WARN level is used.
	Logger *slog.Logger `yaml:"-"`
}

// SourceConfig describes a single input source. Only "file" is supported in
// the library form.
type SourceConfig struct {
	Kind           string `yaml:"kind"`            // "file"
	Path           string `yaml:"path"`            // file path
	Service        string `yaml:"service"`         // optional label; defaults to base filename
	Format         string `yaml:"format"`          // "auto" | "json" | "text"
	StartFrom      string `yaml:"start_from"`      // "beginning" | "end" (default "end")
	FollowRotation bool   `yaml:"follow_rotation"` // default true
}

// AIConfig configures the LLM provider used for cluster analysis.
type AIConfig struct {
	// "logstruct-ai" (default — local OpenAI-compatible endpoint, LM Studio) or "openrouter".
	Provider string `yaml:"provider"`

	// Required for "openrouter".
	APIKey string `yaml:"api_key"`

	// Required for "openrouter" (for example "anthropic/claude-3.5-sonnet").
	// Optional for "logstruct-ai", whatever model the local server has loaded.
	Model string `yaml:"model"`

	// For "logstruct-ai": defaults to http://localhost:1234/v1.
	// For "openrouter": defaults to https://openrouter.ai/api/v1.
	BaseURL string `yaml:"base_url"`

	Timeout    time.Duration `yaml:"timeout"`     // default 45s
	MaxRetries int           `yaml:"max_retries"` // default 2

	// MaxTokens caps the LLM response. Default 2000.
	MaxTokens int `yaml:"max_tokens"`

	// Temperature for the LLM. Default 0.2 for openrouter, forced to 0 for
	// logstruct-ai (small local models give poor structured output otherwise).
	Temperature float64 `yaml:"temperature"`
}

// StorageConfig selects and configures the persistence backend.
type StorageConfig struct {
	// "sqlite" (default), "postgres", or "memory".
	Kind string `yaml:"kind"`

	// SQLitePath is the file path for the SQLite database. Default "./logstruct.db".
	SQLitePath string `yaml:"sqlite_path"`

	// PostgresDSN is the connection string for postgres mode.
	PostgresDSN string `yaml:"postgres_dsn"`
}

// InlineConfig controls the behaviour of ll.Report() and the inline AI path.
type InlineConfig struct {
	// Enabled allows ll.Report() to feed synthesised events into the pipeline.
	Enabled bool `yaml:"enabled"`

	// MinPriority is the cluster priority above which inline AI analysis is
	// triggered automatically. Default 50.
	MinPriority int `yaml:"min_priority"`

	// MaxConcurrent caps the number of background AI requests in flight at once.
	// Default 2.
	MaxConcurrent int `yaml:"max_concurrent"`
}

// ClusterConfig tunes the Drain clustering algorithm (https://jiemingzhu.github.io/pub/pjhe_icws2017.pdf).
type ClusterConfig struct {
	SimilarityThreshold float64       `yaml:"similarity_threshold"` // default 0.4
	MaxDepth            int           `yaml:"max_depth"`            // default 3
	PruneAfter          time.Duration `yaml:"prune_after"`          // default 72h
}

// Fields is a flat key-value map of structured data attached to a Report() call.
// Nested values are JSON-marshalled when stored.
type Fields map[string]any

// envRe matches ${VAR} and ${VAR:-default}.
var envRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// loadYAMLConfig reads a YAML file, expands ${VAR} and ${VAR:-default}, and unmarshal into Config.
func loadYAMLConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("logstruct: read %s: %w", path, err)
	}
	expanded := envRe.ReplaceAllStringFunc(string(raw), func(match string) string {
		sub := envRe.FindStringSubmatch(match)
		name, def := sub[1], sub[2]
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return def
	})

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return Config{}, fmt.Errorf("logstruct: parse %s: %w", path, err)
	}
	return cfg, nil
}

// applyDefaults return defaults data
func (c *Config) applyDefaults() {
	if c.Storage.Kind == "" {
		c.Storage.Kind = "sqlite"
	}
	if c.Storage.SQLitePath == "" {
		c.Storage.SQLitePath = "./logstruct.db"
	}

	if c.AI.Provider == "" {
		c.AI.Provider = "logstruct-ai"
	}
	if c.AI.BaseURL == "" {
		switch c.AI.Provider {
		case "logstruct-ai":
			c.AI.BaseURL = "http://localhost:1234/v1"
		case "openrouter":
			c.AI.BaseURL = "https://openrouter.ai/api/v1"
		}
	}
	if c.AI.Timeout <= 0 {
		c.AI.Timeout = 45 * time.Second
	}
	if c.AI.MaxRetries <= 0 {
		c.AI.MaxRetries = 2
	}
	if c.AI.MaxTokens <= 0 {
		c.AI.MaxTokens = 2000
	}
	// logstruct-ai (local models) must use temperature=0, for others default is 0.2, for stable structured JSON
	if c.AI.Provider == "logstruct-ai" {
		c.AI.Temperature = 0
	} else if c.AI.Temperature == 0 {
		c.AI.Temperature = 0.2
	}

	if c.Inline.MinPriority == 0 {
		c.Inline.MinPriority = 50
	}
	if c.Inline.MaxConcurrent <= 0 {
		c.Inline.MaxConcurrent = 2
	}

	if c.Cluster.SimilarityThreshold == 0 {
		c.Cluster.SimilarityThreshold = 0.4
	}
	if c.Cluster.MaxDepth == 0 {
		c.Cluster.MaxDepth = 3
	}
	if c.Cluster.PruneAfter == 0 {
		c.Cluster.PruneAfter = 72 * time.Hour
	}

	for i := range c.Sources {
		s := &c.Sources[i]
		if s.Format == "" {
			s.Format = "auto"
		}
		if s.StartFrom == "" {
			s.StartFrom = "end"
		}
	}

	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	}
}

// validate returns the first hard error in the config
func (c *Config) validate() error {
	if len(c.Sources) == 0 && !c.Inline.Enabled {
		return errors.New("logstruct: at least one source is required, or set inline.enabled = true")
	}
	for i, s := range c.Sources {
		if s.Kind != "file" {
			return fmt.Errorf("logstruct: sources[%d].kind=%q: only \"file\" is supported", i, s.Kind)
		}
		if s.Path == "" {
			return fmt.Errorf("logstruct: sources[%d].path: must not be empty", i)
		}
	}

	switch c.Storage.Kind {
	case "memory":
		// for testing
	case "postgres":
		if c.Storage.PostgresDSN == "" {
			return errors.New("logstruct: storage.postgres_dsn: required for postgres backend")
		}
	case "sqlite":
		// SQLitePath has a default, no further validation needed.
	default:
		return fmt.Errorf("logstruct: storage.kind=%q: must be one of memory|postgres|sqlite", c.Storage.Kind)
	}

	switch c.AI.Provider {
	case "logstruct-ai":
		// no API key required (in default mode on 04.2026)
	case "openrouter":
		if c.AI.APIKey == "" {
			return errors.New("logstruct: ai.api_key: required for openrouter")
		}
		if c.AI.Model == "" {
			return errors.New("logstruct: ai.model: required for openrouter (e.g. anthropic/claude-3.5-sonnet)")
		}
	case "fake":
		// only for example
	default:
		return fmt.Errorf("logstruct: ai.provider=%q: must be \"logstruct-ai\" or \"openrouter\"", c.AI.Provider)
	}

	if t := c.Cluster.SimilarityThreshold; t < 0 || t > 1 {
		return fmt.Errorf("logstruct: cluster.similarity_threshold=%g: must be in [0,1]", t)
	}

	return nil
}

// toInternal translates the public Config into the internal config.Config used by the existing internal packages.
func (c *Config) toInternal() *config.Config {
	internal := &config.Config{
		Storage: config.StorageConfig{
			Kind:         c.Storage.Kind,
			DSN:          c.Storage.PostgresDSN,
			MaxOpenConns: 20,
			MaxIdleConns: 4,
		},
		Cluster: config.ClusterConfig{
			SimilarityThreshold: c.Cluster.SimilarityThreshold,
			MaxDepth:            c.Cluster.MaxDepth,
			MaxChildrenPerNode:  100,
			MaxTemplatesPerLeaf: 50,
			PruneAfter:          config.Duration(c.Cluster.PruneAfter),
		},
		Score: config.ScoreConfig{
			Window: config.Duration(time.Minute),
			Weights: config.ScoreWeights{
				LevelFatal: 40, LevelError: 25, LevelWarn: 10,
				Frequency: 1.0, Burst: 15, Novelty: 20, Rarity: 10, CrossService: 15,
			},
		},
		Analyze: config.AnalyzeConfig{
			Enabled:           true,
			PriorityThreshold: c.Inline.MinPriority,
			MaxConcurrent:     c.Inline.MaxConcurrent,
			CacheTTL:          config.Duration(15 * time.Minute),
			Window:            config.Duration(10 * time.Minute),
		},
		LLM: config.LLMConfig{
			Provider:    c.AI.Provider,
			APIKey:      c.AI.APIKey,
			BaseURL:     c.AI.BaseURL,
			Model:       c.AI.Model,
			Timeout:     config.Duration(c.AI.Timeout),
			MaxRetries:  c.AI.MaxRetries,
			MaxTokens:   c.AI.MaxTokens,
			Temperature: c.AI.Temperature,
		},
	}

	internal.Sources = make([]config.SourceConfig, 0, len(c.Sources))
	for i, s := range c.Sources {
		name := s.Service
		if name == "" {
			name = fmt.Sprintf("source-%d", i)
		}
		internal.Sources = append(internal.Sources, config.SourceConfig{
			Name:    name,
			Kind:    s.Kind,
			Service: s.Service,
			Format:  s.Format,
			File: &config.FileSourceConfig{
				Path:           s.Path,
				FollowRotation: s.FollowRotation,
				StartFrom:      s.StartFrom,
			},
		})
	}

	return internal
}
