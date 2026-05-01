package analyze_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tragidra/logstruct/internal/analyze"
	"github.com/Tragidra/logstruct/internal/config"
	"github.com/Tragidra/logstruct/internal/llm"
	"github.com/Tragidra/logstruct/internal/llm/fake"
	"github.com/Tragidra/logstruct/internal/storage"
	"github.com/Tragidra/logstruct/model"
)

// for prompt rendering
func TestRenderUserPrompt_IncludesAllSections(t *testing.T) {
	out, err := analyze.RenderUserPrompt(analyze.PromptData{
		ClusterID:      "cls-abc",
		Template:       "connection to <*> refused",
		CountTotal:     847,
		CountInWindow:  312,
		WindowDuration: "5m0s",
		Services:       []string{"api-gateway", "order-service"},
		LevelBreakdown: map[model.Level]int64{model.LevelError: 312},
		TimeSpan:       "2026-04-19T14:22:00Z to 2026-04-19T14:27:00Z",
		Flags:          []string{"burst", "cross-service"},
		Examples: []string{
			"ERROR connection to payment:8080 refused",
			"ERROR connection to payment:8080 refused (retry 2)",
		},
		Neighbors: []analyze.NeighborCluster{
			{ID: "cls-xyz", Template: "circuit breaker opened for <*>", Priority: 82, CountInWindow: 45, Services: []string{"api-gateway"}},
		},
	})
	require.NoError(t, err)

	assert.Contains(t, out, "Cluster ID: cls-abc")
	assert.Contains(t, out, "connection to <*> refused")
	assert.Contains(t, out, "Total events seen: 847")
	assert.Contains(t, out, "api-gateway, order-service")
	assert.Contains(t, out, "error=312")
	assert.Contains(t, out, "burst, cross-service")
	assert.Contains(t, out, "ERROR connection to payment:8080 refused")
	assert.Contains(t, out, "circuit breaker opened for <*>")
	assert.Contains(t, out, "priority=82")
	assert.Contains(t, out, "id=cls-xyz")
}

func TestRenderUserPrompt_NoNeighborsShowsNone(t *testing.T) {
	out, err := analyze.RenderUserPrompt(analyze.PromptData{
		Template: "x",
	})
	require.NoError(t, err)
	assert.Contains(t, out, "(none)")
}

func TestSchema_IsValidJSON(t *testing.T) {
	var v map[string]interface{}
	require.NoError(t, json.Unmarshal(analyze.ClusterAnalysisSchema, &v))
	assert.Equal(t, "cluster_analysis", v["name"])
	assert.NotNil(t, v["schema"])
}

func TestAnalyze_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	cluster := newCluster("cluster-1", "fp-1")
	repo.clusters[cluster.ID] = cluster

	provider := fake.New()
	provider.SetResponse(llm.Response{
		Content: `{
			"summary": "Two services lost connectivity to payment-service.",
			"severity": "critical",
			"root_cause_hypothesis": "payment-service is likely down or unreachable.",
			"suggested_actions": ["Check payment-service pod status.", "Verify network path to payment-service."],
			"related_cluster_ids": ["cluster-2"],
			"confidence": 0.85
		}`,
		Model:        "openai/gpt-4o-mini",
		InputTokens:  120,
		OutputTokens: 80,
		LatencyMs:    1200,
	})

	a := analyze.New(config.AnalyzeConfig{}, provider, repo, slog.Default())

	an, err := a.Analyze(context.Background(), cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, "Two services lost connectivity to payment-service.", an.Summary)
	assert.Equal(t, model.SeverityCritical, an.Severity)
	assert.Len(t, an.SuggestedActions, 2)
	assert.InDelta(t, 0.85, an.Confidence, 0.01)
	assert.Equal(t, "openai/gpt-4o-mini", an.ModelUsed)
	assert.Equal(t, 120, an.TokensInput)
	assert.Equal(t, 80, an.TokensOutput)

	require.Len(t, repo.savedAnalyses, 1)
	assert.Equal(t, cluster.ID, repo.savedAnalyses[0].ClusterID)
}

func TestAnalyze_CacheReturnsSameInstance(t *testing.T) {
	repo := newFakeRepo()
	cluster := newCluster("cluster-2", "fp-2")
	repo.clusters[cluster.ID] = cluster

	provider := fake.New()
	provider.SetResponse(llm.Response{
		Content: `{"summary":"Health check passing.","severity":"info","root_cause_hypothesis":"Routine health probe traffic.","suggested_actions":["No action needed; ignore."],"confidence":0.95}`,
	})

	a := analyze.New(config.AnalyzeConfig{}, provider, repo, slog.Default())

	first, err := a.Analyze(context.Background(), cluster.ID)
	require.NoError(t, err)
	second, err := a.Analyze(context.Background(), cluster.ID)
	require.NoError(t, err)

	assert.Same(t, first, second, "cached analysis should be returned by pointer identity")
	assert.Len(t, provider.Calls(), 1, "LLM should have been hit only once")
}

func TestAnalyze_LLMErrorReturnsFallback(t *testing.T) {
	repo := newFakeRepo()
	cluster := newCluster("cluster-3", "fp-3")
	repo.clusters[cluster.ID] = cluster

	provider := fake.New()
	provider.SetError(errors.New("boom"))

	a := analyze.New(config.AnalyzeConfig{}, provider, repo, slog.Default())

	an, err := a.Analyze(context.Background(), cluster.ID)
	require.NoError(t, err, "Analyze never bubbles up LLM errors")
	assert.Contains(t, an.Summary, "Analysis failed")
	assert.Equal(t, model.SeverityUnknown, an.Severity)
}

func TestAnalyze_InvalidJSONReturnsFallback(t *testing.T) {
	repo := newFakeRepo()
	cluster := newCluster("cluster-4", "fp-4")
	repo.clusters[cluster.ID] = cluster

	provider := fake.New()
	provider.SetResponse(llm.Response{Content: "not json at all"})

	a := analyze.New(config.AnalyzeConfig{}, provider, repo, slog.Default())

	an, err := a.Analyze(context.Background(), cluster.ID)
	require.NoError(t, err)
	assert.Contains(t, an.Summary, "malformed JSON")
}

func TestAnalyze_MissingFieldsRejected(t *testing.T) {
	repo := newFakeRepo()
	cluster := newCluster("cluster-5", "fp-5")
	repo.clusters[cluster.ID] = cluster

	provider := fake.New()
	provider.SetResponse(llm.Response{Content: `{"summary":"only summary"}`})

	a := analyze.New(config.AnalyzeConfig{}, provider, repo, slog.Default())

	an, err := a.Analyze(context.Background(), cluster.ID)
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(an.Summary), "malformed")
}

func TestAnalyze_PromptIncludesClusterTemplate(t *testing.T) {
	repo := newFakeRepo()
	cluster := newCluster("cluster-6", "fp-6")
	cluster.Template = "user <*> failed login from <*>"
	repo.clusters[cluster.ID] = cluster

	provider := fake.New()
	provider.SetResponse(llm.Response{
		Content: `{"summary":"OK now, looks benign.","severity":"info","root_cause_hypothesis":"Just a probe likely.","suggested_actions":["Ignore for now please."],"confidence":0.5}`,
	})

	a := analyze.New(config.AnalyzeConfig{}, provider, repo, slog.Default())
	_, err := a.Analyze(context.Background(), cluster.ID)
	require.NoError(t, err)

	calls := provider.Calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "system", calls[0].Messages[0].Role)
	assert.Contains(t, calls[0].Messages[1].Content, "user <*> failed login from <*>")
	assert.Contains(t, calls[0].Messages[1].Content, "Cluster ID: cluster-6")
	assert.NotNil(t, calls[0].JSONSchema)
}

func TestParseAnalysisJSON_Validation(t *testing.T) {
	validJSON := func(overrides map[string]any) string {
		base := map[string]any{
			"summary":               "payment service is timing out now",
			"severity":              "warning",
			"root_cause_hypothesis": "likely downstream DB congestion",
			"suggested_actions":     []string{"Check DB latency metrics", "Inspect pod logs"},
			"related_cluster_ids":   []string{},
			"confidence":            0.75,
		}
		for k, v := range overrides {
			base[k] = v
		}
		b, _ := json.Marshal(base)
		return string(b)
	}

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "happy path",
			input:   validJSON(nil),
			wantErr: "",
		},
		{
			name:    "invalid JSON",
			input:   "not json",
			wantErr: "decode",
		},
		{
			name:    "missing summary",
			input:   validJSON(map[string]any{"summary": ""}),
			wantErr: "missing required fields",
		},
		{
			name:    "missing severity",
			input:   validJSON(map[string]any{"severity": ""}),
			wantErr: "missing required fields",
		},
		{
			name:    "invalid severity",
			input:   validJSON(map[string]any{"severity": "unknown"}),
			wantErr: "invalid severity",
		},
		{
			name:    "confidence too high",
			input:   validJSON(map[string]any{"confidence": 1.5}),
			wantErr: "confidence",
		},
		{
			name:    "confidence negative",
			input:   validJSON(map[string]any{"confidence": -0.1}),
			wantErr: "confidence",
		},
		{
			name:    "summary too short",
			input:   validJSON(map[string]any{"summary": "bad"}),
			wantErr: "summary too short",
		},
		{
			name:    "empty suggested_actions",
			input:   validJSON(map[string]any{"suggested_actions": []string{}}),
			wantErr: "suggested_actions must not be empty",
		},
		{
			name:    "action too short",
			input:   validJSON(map[string]any{"suggested_actions": []string{"act"}}),
			wantErr: "suggested_actions[0] too short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := analyze.ParseAnalysisJSON(tt.input)
			if tt.wantErr == "" {
				require.NoError(t, err)
				require.NotNil(t, got)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestWorkerPool_ProcessesSubmittedJobs(t *testing.T) {
	repo := newFakeRepo()
	cluster := newCluster("cluster-7", "fp-7")
	repo.clusters[cluster.ID] = cluster

	provider := fake.New()
	provider.SetResponse(llm.Response{
		Content: `{"summary":"All good here.","severity":"info","root_cause_hypothesis":"Routine traffic looks fine.","suggested_actions":["No action needed."],"confidence":0.9}`,
	})

	a := analyze.New(config.AnalyzeConfig{}, provider, repo, slog.Default())
	pool := analyze.NewWorkerPool(a, 2, 4, slog.Default())

	assert.True(t, pool.Submit(cluster.ID))

	// Give workers time to drain.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(provider.Calls()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	pool.Close()
	assert.GreaterOrEqual(t, len(provider.Calls()), 1)
}

type fakeRepo struct {
	mu            sync.Mutex
	clusters      map[string]model.Cluster
	savedAnalyses []model.Analysis
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{clusters: make(map[string]model.Cluster)}
}

func (f *fakeRepo) GetCluster(_ context.Context, id string) (model.Cluster, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.clusters[id]
	if !ok {
		return model.Cluster{}, storage.ErrNotFound
	}
	return c, nil
}

func (f *fakeRepo) SaveAnalysis(_ context.Context, a model.Analysis) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.savedAnalyses = append(f.savedAnalyses, a)
	return nil
}

func (f *fakeRepo) UpsertCluster(context.Context, model.Cluster) error {
	return storage.ErrNotImplemented
}
func (f *fakeRepo) ListClusters(context.Context, storage.ClusterFilter) ([]model.Cluster, int64, error) {
	return nil, 0, storage.ErrNotImplemented
}
func (f *fakeRepo) PruneStaleClusters(context.Context, time.Time) (int64, error) {
	return 0, storage.ErrNotImplemented
}
func (f *fakeRepo) SaveEvent(context.Context, model.LogEvent, string) error {
	return storage.ErrNotImplemented
}
func (f *fakeRepo) ListEventsByCluster(context.Context, string, storage.EventFilter) ([]model.LogEvent, error) {
	return nil, storage.ErrNotImplemented
}
func (f *fakeRepo) LatestAnalysisForCluster(context.Context, string) (*model.Analysis, error) {
	return nil, storage.ErrNotImplemented
}
func (f *fakeRepo) ListRecentAnalyses(context.Context, int) ([]model.Analysis, error) {
	return nil, storage.ErrNotImplemented
}
func (f *fakeRepo) Ping(context.Context) error { return nil }
func (f *fakeRepo) Close() error               { return nil }

func newCluster(id, fp string) model.Cluster {
	return model.Cluster{
		ID:             id,
		Fingerprint:    fp,
		Template:       "test template <*>",
		Services:       []string{"svc-a"},
		Levels:         map[model.Level]int64{model.LevelError: 5},
		Count:          5,
		FirstSeen:      time.Now().Add(-1 * time.Hour),
		LastSeen:       time.Now(),
		ExamplesSample: []string{"example log line"},
	}
}
