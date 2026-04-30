// Package config handles loading and validating logsense configuration, like config.go in root repo
package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration to support YAML unmarshaling from strings like "15s", "1m", "2h".
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	parsed, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	*d = Duration(parsed)
	return nil
}

// D returns the underlying time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// Config is the top-level configuration structure.
type Config struct {
	API     APIConfig      `yaml:"api"`
	Storage StorageConfig  `yaml:"storage"`
	Sources []SourceConfig `yaml:"sources"`
	Cluster ClusterConfig  `yaml:"cluster"`
	Score   ScoreConfig    `yaml:"score"`
	Analyze AnalyzeConfig  `yaml:"analyze"`
	LLM     LLMConfig      `yaml:"llm"`
	Log     LogConfig      `yaml:"log"`
}

// APIConfig configures the HTTP API server.
type APIConfig struct {
	Addr           string   `yaml:"addr"`
	AllowedOrigins []string `yaml:"allowed_origins"`
	ReadTimeout    Duration `yaml:"read_timeout"`
	WriteTimeout   Duration `yaml:"write_timeout"`
}

// StorageConfig configures the persistence backend.
type StorageConfig struct {
	Kind         string `yaml:"kind"`
	DSN          string `yaml:"dsn"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
}

// SourceConfig describes a single ingestion source.
type SourceConfig struct {
	Name    string            `yaml:"name"`
	Kind    string            `yaml:"kind"`
	Service string            `yaml:"service"`
	Format  string            `yaml:"format"`
	File    *FileSourceConfig `yaml:"file,omitempty"`
}

// FileSourceConfig configures a file-tail source.
type FileSourceConfig struct {
	Path           string `yaml:"path"`
	FollowRotation bool   `yaml:"follow_rotation"`
	StartFrom      string `yaml:"start_from"` // "beginning" | "end"
}

// ClusterConfig configures the Drain clustering algorithm.
type ClusterConfig struct {
	SimilarityThreshold float64  `yaml:"similarity_threshold"`
	MaxDepth            int      `yaml:"max_depth"`
	MaxChildrenPerNode  int      `yaml:"max_children_per_node"`
	MaxTemplatesPerLeaf int      `yaml:"max_templates_per_leaf"`
	PruneAfter          Duration `yaml:"prune_after"`
}

// ScoreConfig configures cluster priority scoring.
type ScoreConfig struct {
	Window  Duration     `yaml:"window"`
	Weights ScoreWeights `yaml:"weights"`
}

// ScoreWeights holds the formula coefficients for scoring.
type ScoreWeights struct {
	LevelFatal   int     `yaml:"level_fatal"`
	LevelError   int     `yaml:"level_error"`
	LevelWarn    int     `yaml:"level_warn"`
	LevelInfo    int     `yaml:"level_info"`
	Frequency    float64 `yaml:"frequency"`
	Burst        float64 `yaml:"burst"`
	Novelty      float64 `yaml:"novelty"`
	Rarity       float64 `yaml:"rarity"`
	CrossService float64 `yaml:"cross_service"`
}

// AnalyzeConfig configures LLM-backed cluster analysis.
type AnalyzeConfig struct {
	Enabled           bool     `yaml:"enabled"`
	PriorityThreshold int      `yaml:"priority_threshold"`
	MaxConcurrent     int      `yaml:"max_concurrent"`
	CacheTTL          Duration `yaml:"cache_ttl"`
	Window            Duration `yaml:"window"`
}

// LLMConfig configures the LLM provider connection.
type LLMConfig struct {
	Provider    string   `yaml:"provider"`
	APIKey      string   `yaml:"api_key"`
	BaseURL     string   `yaml:"base_url"`
	Model       string   `yaml:"model"`
	Timeout     Duration `yaml:"timeout"`
	MaxRetries  int      `yaml:"max_retries"`
	MaxTokens   int      `yaml:"max_tokens"`
	Temperature float64  `yaml:"temperature"`
}

// LogConfig configures logsense's own structured logger.
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}
