package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var envRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// Load reads a YAML config file, expands env vars, applies defaults, and validates.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	expanded := expandEnv(string(raw))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	return &cfg, nil
}

// expandEnv replaces ${VAR} and ${VAR:-default} with environment variable values.
func expandEnv(s string) string {
	return envRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := envRe.FindStringSubmatch(match)
		name, def := sub[1], sub[2]
		if val, ok := os.LookupEnv(name); ok {
			return val
		}
		return def
	})
}

func applyDefaults(cfg *Config) {
	if cfg.API.Addr == "" {
		cfg.API.Addr = ":8080"
	}
	if cfg.API.ReadTimeout == 0 {
		cfg.API.ReadTimeout = Duration(15 * time.Second)
	}
	if cfg.API.WriteTimeout == 0 {
		cfg.API.WriteTimeout = Duration(15 * time.Second)
	}

	if cfg.Storage.Kind == "" {
		cfg.Storage.Kind = "postgres"
	}
	if cfg.Storage.MaxOpenConns == 0 {
		cfg.Storage.MaxOpenConns = 20
	}
	if cfg.Storage.MaxIdleConns == 0 {
		cfg.Storage.MaxIdleConns = 4
	}

	for i := range cfg.Sources {
		if cfg.Sources[i].Format == "" {
			cfg.Sources[i].Format = "auto"
		}
		if cfg.Sources[i].File != nil && cfg.Sources[i].File.StartFrom == "" {
			cfg.Sources[i].File.StartFrom = "end"
		}
	}

	if cfg.Cluster.SimilarityThreshold == 0 {
		cfg.Cluster.SimilarityThreshold = 0.4
	}
	if cfg.Cluster.MaxDepth == 0 {
		cfg.Cluster.MaxDepth = 3
	}
	if cfg.Cluster.MaxChildrenPerNode == 0 {
		cfg.Cluster.MaxChildrenPerNode = 100
	}
	if cfg.Cluster.MaxTemplatesPerLeaf == 0 {
		cfg.Cluster.MaxTemplatesPerLeaf = 50
	}
	if cfg.Cluster.PruneAfter == 0 {
		cfg.Cluster.PruneAfter = Duration(72 * time.Hour)
	}

	if cfg.Score.Window == 0 {
		cfg.Score.Window = Duration(time.Minute)
	}
	w := &cfg.Score.Weights
	if w.LevelFatal == 0 {
		w.LevelFatal = 40
	}
	if w.LevelError == 0 {
		w.LevelError = 25
	}
	if w.LevelWarn == 0 {
		w.LevelWarn = 10
	}
	if w.Frequency == 0 {
		w.Frequency = 1.0
	}
	if w.Burst == 0 {
		w.Burst = 15
	}
	if w.Novelty == 0 {
		w.Novelty = 20
	}
	if w.Rarity == 0 {
		w.Rarity = 10
	}
	if w.CrossService == 0 {
		w.CrossService = 15
	}

	if cfg.Analyze.PriorityThreshold == 0 {
		cfg.Analyze.PriorityThreshold = 40
	}
	if cfg.Analyze.MaxConcurrent == 0 {
		cfg.Analyze.MaxConcurrent = 4
	}
	if cfg.Analyze.CacheTTL == 0 {
		cfg.Analyze.CacheTTL = Duration(15 * time.Minute)
	}
	if cfg.Analyze.Window == 0 {
		cfg.Analyze.Window = Duration(10 * time.Minute)
	}

	if cfg.LLM.Timeout == 0 {
		cfg.LLM.Timeout = Duration(30 * time.Second)
	}
	if cfg.LLM.MaxRetries == 0 {
		cfg.LLM.MaxRetries = 2
	}
	if cfg.LLM.MaxTokens == 0 {
		cfg.LLM.MaxTokens = 1500
	}
	if cfg.LLM.Temperature == 0 {
		cfg.LLM.Temperature = 0.2
	}

	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "json"
	}
}

func validate(cfg *Config) error {
	t := cfg.Cluster.SimilarityThreshold
	if t < 0 || t > 1 {
		return fmt.Errorf("cluster.similarity_threshold: must be in [0, 1], got %g", t)
	}

	if cfg.LLM.Provider != "" {
		needsKey := cfg.LLM.Provider != "logstruct-ai" || cfg.LLM.BaseURL == ""
		if needsKey && cfg.LLM.APIKey == "" {
			return fmt.Errorf("llm.api_key: required when provider is %q", cfg.LLM.Provider)
		}
	}

	if cfg.LLM.Provider != "" && cfg.LLM.Model == "" {
		return fmt.Errorf("llm.model: required when provider is set")
	}

	valid := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !valid[strings.ToLower(cfg.Log.Level)] {
		return fmt.Errorf("log.level: invalid value %q (want debug|info|warn|error)", cfg.Log.Level)
	}

	return nil
}
