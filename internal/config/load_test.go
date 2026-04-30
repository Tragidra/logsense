package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Minimal(t *testing.T) {
	cfg, err := Load("testdata/minimal.yaml")
	require.NoError(t, err)
	assert.Equal(t, "postgres://loglens:loglens@localhost:5432/loglens?sslmode=disable", cfg.Storage.DSN)
}

func TestLoad_Minimal_Defaults(t *testing.T) {
	cfg, err := Load("testdata/minimal.yaml")
	require.NoError(t, err)

	assert.Equal(t, ":8080", cfg.API.Addr)
	assert.Equal(t, Duration(15*time.Second), cfg.API.ReadTimeout)
	assert.Equal(t, Duration(15*time.Second), cfg.API.WriteTimeout)
	assert.Equal(t, "postgres", cfg.Storage.Kind)
	assert.Equal(t, 20, cfg.Storage.MaxOpenConns)
	assert.Equal(t, 4, cfg.Storage.MaxIdleConns)
	assert.Equal(t, "auto", cfg.Sources[0].Format)
	assert.Equal(t, "end", cfg.Sources[0].File.StartFrom)
	assert.InDelta(t, 0.4, cfg.Cluster.SimilarityThreshold, 1e-9)
	assert.Equal(t, 3, cfg.Cluster.MaxDepth)
	assert.Equal(t, 100, cfg.Cluster.MaxChildrenPerNode)
	assert.Equal(t, 50, cfg.Cluster.MaxTemplatesPerLeaf)
	assert.Equal(t, Duration(72*time.Hour), cfg.Cluster.PruneAfter)
	assert.Equal(t, Duration(time.Minute), cfg.Score.Window)
	assert.Equal(t, 40, cfg.Score.Weights.LevelFatal)
	assert.Equal(t, 25, cfg.Score.Weights.LevelError)
	assert.Equal(t, 10, cfg.Score.Weights.LevelWarn)
	assert.Equal(t, 40, cfg.Analyze.PriorityThreshold)
	assert.Equal(t, 4, cfg.Analyze.MaxConcurrent)
	assert.Equal(t, Duration(15*time.Minute), cfg.Analyze.CacheTTL)
	assert.Equal(t, Duration(10*time.Minute), cfg.Analyze.Window)
	assert.Equal(t, Duration(30*time.Second), cfg.LLM.Timeout)
	assert.Equal(t, 2, cfg.LLM.MaxRetries)
	assert.Equal(t, 1500, cfg.LLM.MaxTokens)
	assert.InDelta(t, 0.2, cfg.LLM.Temperature, 1e-9)
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "json", cfg.Log.Format)
}

func TestLoad_Full(t *testing.T) {
	cfg, err := Load("testdata/full.yaml")
	require.NoError(t, err)

	assert.Equal(t, ":9090", cfg.API.Addr)
	assert.Equal(t, Duration(20*time.Second), cfg.API.ReadTimeout)
	assert.Equal(t, 30, cfg.Storage.MaxOpenConns)
	require.Len(t, cfg.Sources, 2)
	assert.Equal(t, "nginx-prod", cfg.Sources[0].Name)
	assert.Equal(t, "nginx", cfg.Sources[0].Format)
	assert.Equal(t, "beginning", cfg.Sources[0].File.StartFrom)
	assert.InDelta(t, 0.5, cfg.Cluster.SimilarityThreshold, 1e-9)
	assert.Equal(t, Duration(48*time.Hour), cfg.Cluster.PruneAfter)
	assert.Equal(t, Duration(2*time.Minute), cfg.Score.Window)
	assert.Equal(t, 50, cfg.Score.Weights.LevelFatal)
	assert.True(t, cfg.Analyze.Enabled)
	assert.Equal(t, 50, cfg.Analyze.PriorityThreshold)
	assert.Equal(t, "openrouter", cfg.LLM.Provider)
	assert.Equal(t, "test-key-abc123", cfg.LLM.APIKey)
	assert.Equal(t, Duration(45*time.Second), cfg.LLM.Timeout)
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, "text", cfg.Log.Format)
}

func TestLoad_EnvExpansion(t *testing.T) {
	t.Run("explicit var set", func(t *testing.T) {
		t.Setenv("TEST_API_KEY", "my-secret-key")
		t.Setenv("TEST_DSN", "postgres://custom:pass@db:5432/test?sslmode=disable")

		cfg, err := Load("testdata/env_expansion.yaml")
		require.NoError(t, err)
		assert.Equal(t, "postgres://custom:pass@db:5432/test?sslmode=disable", cfg.Storage.DSN)
		assert.Equal(t, "my-secret-key", cfg.LLM.APIKey)
	})

	t.Run("default fallback when var unset", func(t *testing.T) {
		orig, existed := os.LookupEnv("TEST_DSN")
		require.NoError(t, os.Unsetenv("TEST_DSN"))
		t.Cleanup(func() {
			if existed {
				os.Setenv("TEST_DSN", orig)
			}
		})
		t.Setenv("TEST_API_KEY", "fallback-key")

		cfg, err := Load("testdata/env_expansion.yaml")
		require.NoError(t, err)
		assert.Equal(t, "postgres://default:default@localhost:5432/loglens?sslmode=disable", cfg.Storage.DSN)
	})
}

func TestLoad_InvalidSimilarityThreshold(t *testing.T) {
	_, err := Load("testdata/invalid_similarity.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cluster.similarity_threshold")
}

func TestLoad_InvalidLLMNoAPIKey(t *testing.T) {
	_, err := Load("testdata/invalid_llm_no_key.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm.api_key")
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("testdata/nonexistent.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent.yaml")
}

func TestExpandEnv(t *testing.T) {
	tests := []struct {
		name  string
		input string
		env   map[string]string
		want  string
	}{
		{
			name:  "simple var",
			input: "dsn: ${DB_DSN}",
			env:   map[string]string{"DB_DSN": "postgres://localhost/test"},
			want:  "dsn: postgres://localhost/test",
		},
		{
			name:  "default used when unset",
			input: "${MISSING:-fallback}",
			env:   map[string]string{},
			want:  "fallback",
		},
		{
			name:  "explicit value overrides default",
			input: "${KEY:-default}",
			env:   map[string]string{"KEY": "actual"},
			want:  "actual",
		},
		{
			name:  "empty string overrides default",
			input: "${EMPTY:-default}",
			env:   map[string]string{"EMPTY": ""},
			want:  "",
		},
		{
			name:  "no vars unchanged",
			input: "plain text",
			env:   map[string]string{},
			want:  "plain text",
		},
		{
			name:  "multiple vars",
			input: "${A} and ${B:-b_default}",
			env:   map[string]string{"A": "alpha"},
			want:  "alpha and b_default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got := expandEnv(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
