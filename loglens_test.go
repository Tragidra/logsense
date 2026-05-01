package logstruct

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tragidra/logstruct/internal/llm"
	"github.com/Tragidra/logstruct/internal/llm/fake"
	"github.com/Tragidra/logstruct/internal/storage"
)

func newTestLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestNew_RejectsEmptyConfig(t *testing.T) {
	_, err := New(Config{})
	require.Error(t, err, "expected error when no sources and inline disabled")
}

func TestNew_RejectsUnknownSourceKind(t *testing.T) {
	_, err := New(Config{
		Sources: []SourceConfig{{Kind: "kafka", Path: "irrelevant"}},
		AI:      AIConfig{Provider: "fake"},
		Storage: StorageConfig{Kind: "memory"},
	})
	require.Error(t, err)
}

func TestNew_OpenrouterRequiresAPIKey(t *testing.T) {
	_, err := New(Config{
		Inline:  InlineConfig{Enabled: true},
		AI:      AIConfig{Provider: "openrouter", Model: "anthropic/claude-3.5-sonnet"},
		Storage: StorageConfig{Kind: "memory"},
	})
	require.Error(t, err)
}

// TestSmoke_InlineReportCreatesCluster is the R1 smoke test:
// - Construct logstruct with in-memory storage, inline mode, fake AI.
// - Start, Report a few errors, Close.
// - Verify a cluster was created in the repo.
func TestSmoke_InlineReportCreatesCluster(t *testing.T) {
	logger := newTestLogger(t)
	ll, err := New(Config{
		Inline:  InlineConfig{Enabled: true, MinPriority: 100}, // never trigger AI
		AI:      AIConfig{Provider: "fake"},
		Storage: StorageConfig{Kind: "memory"},
		Logger:  logger,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ll.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, ll.Start(ctx))

	for i := 0; i < 5; i++ {
		ll.Report(ctx, errors.New("payment service timeout for order 12345"), Fields{"order_id": "12345"})
	}

	require.Eventually(t, func() bool {
		clusters, _, err := ll.repo.ListClusters(ctx, storage.ClusterFilter{Limit: 10})
		if err != nil || len(clusters) == 0 {
			return false
		}
		var total int64
		for _, c := range clusters {
			total += c.Count
		}
		return total >= 5
	}, 3*time.Second, 50*time.Millisecond, "expected at least one cluster with >= 5 events")

	require.Equal(t, int64(0), ll.Stats().Dropped, "no events should have been dropped under light load")
}

func TestReport_NoOpWhenInlineDisabled(t *testing.T) {
	logger := newTestLogger(t)
	ll, err := New(Config{
		Sources: []SourceConfig{{Kind: "file", Path: "/dev/null"}},
		Inline:  InlineConfig{Enabled: false},
		AI:      AIConfig{Provider: "fake"},
		Storage: StorageConfig{Kind: "memory"},
		Logger:  logger,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ll.Close() })

	ctx := context.Background()
	require.NoError(t, ll.Start(ctx))

	ll.Report(ctx, errors.New("ignored"), nil)
	require.Equal(t, int64(0), ll.Stats().Dropped)
}

// TestSmoke_InlineReportSQLitePersists exercises the default SQLite backend
// (R2): events flow through the pipeline and a cluster row exists in the
// .db file after Close().
func TestSmoke_InlineReportSQLitePersists(t *testing.T) {
	logger := newTestLogger(t)
	dbPath := filepath.Join(t.TempDir(), "logstruct.db")

	ll, err := New(Config{
		Inline:  InlineConfig{Enabled: true, MinPriority: 100},
		AI:      AIConfig{Provider: "fake"},
		Storage: StorageConfig{Kind: "sqlite", SQLitePath: dbPath},
		Logger:  logger,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, ll.Start(ctx))

	for i := 0; i < 4; i++ {
		ll.Report(ctx, errors.New("payment service timeout"), Fields{"order_id": "abc"})
	}

	require.Eventually(t, func() bool {
		clusters, _, err := ll.repo.ListClusters(ctx, storage.ClusterFilter{Limit: 10})
		return err == nil && len(clusters) > 0 && clusters[0].Count >= 4
	}, 3*time.Second, 50*time.Millisecond)

	require.NoError(t, ll.Close())

	// Reopen the SQLite file: data should still be there.
	ll2, err := New(Config{
		Inline:  InlineConfig{Enabled: true},
		AI:      AIConfig{Provider: "fake"},
		Storage: StorageConfig{Kind: "sqlite", SQLitePath: dbPath},
		Logger:  logger,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ll2.Close() })

	clusters, _, err := ll2.repo.ListClusters(ctx, storage.ClusterFilter{Limit: 10})
	require.NoError(t, err)
	require.NotEmpty(t, clusters, "data should persist across restarts")
}

// TestSmoke_MultiFileSourcesIngest is the R6 smoke test:
// three file sources tail in parallel; lines from every file must end up in
// clusters via the pipeline.
func TestSmoke_MultiFileSourcesIngest(t *testing.T) {
	dir := t.TempDir()
	const linesPerFile = 5

	type fileSpec struct {
		path    string
		service string
		message string
	}
	specs := []fileSpec{
		{filepath.Join(dir, "auth.log"), "auth-svc", "auth: failed login for user alice"},
		{filepath.Join(dir, "db.log"), "db-svc", "db: query timeout for select users"},
		{filepath.Join(dir, "worker.log"), "worker-svc", "worker: job 42 retries exhausted"},
	}

	// Pre-create the files so tail can open them. Empty is fine; we'll write
	// after the source is streaming so StartFrom doesn't matter for correctness.
	for _, s := range specs {
		f, err := os.Create(s.path)
		require.NoError(t, err)
		require.NoError(t, f.Close())
	}

	sources := make([]SourceConfig, len(specs))
	for i, s := range specs {
		sources[i] = SourceConfig{
			Kind:      "file",
			Path:      s.path,
			Service:   s.service,
			StartFrom: "beginning",
		}
	}

	ll, err := New(Config{
		Sources: sources,
		AI:      AIConfig{Provider: "fake"},
		Storage: StorageConfig{Kind: "memory"},
		Logger:  newTestLogger(t),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ll.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, ll.Start(ctx))

	// Give tail a beat to open and watch each file.
	time.Sleep(150 * time.Millisecond)

	// Write to all three files concurrently to exercise the multi-source path.
	var wg sync.WaitGroup
	for _, s := range specs {
		wg.Add(1)
		go func(s fileSpec) {
			defer wg.Done()
			f, err := os.OpenFile(s.path, os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				t.Errorf("open %s: %v", s.path, err)
				return
			}
			defer f.Close()
			for i := 0; i < linesPerFile; i++ {
				line := fmt.Sprintf(`{"level":"ERROR","service":%q,"message":%q,"seq":%d}`,
					s.service, s.message, i)
				if _, err := fmt.Fprintln(f, line); err != nil {
					t.Errorf("write %s: %v", s.path, err)
					return
				}
			}
		}(s)
	}
	wg.Wait()

	wantTotal := int64(len(specs) * linesPerFile)

	require.Eventually(t, func() bool {
		clusters, _, err := ll.repo.ListClusters(ctx, storage.ClusterFilter{Limit: 50})
		if err != nil || len(clusters) == 0 {
			return false
		}
		var total int64
		seen := make(map[string]bool)
		for _, c := range clusters {
			total += c.Count
			for _, svc := range c.Services {
				seen[svc] = true
			}
		}
		return total >= wantTotal && len(seen) == len(specs)
	}, 5*time.Second, 50*time.Millisecond,
		"expected %d events across clusters covering all 3 services", wantTotal)

	require.Equal(t, int64(0), ll.Stats().Dropped, "no events should drop under light load")
}

// --- R7: inline mode, AnalyzeNow, background AI, drop counter ---

// validAnalysisJSON is a canned LLM response that satisfies parseAnalysisJSON.
const validAnalysisJSON = `{
	"summary": "payment service is timing out",
	"severity": "warning",
	"root_cause_hypothesis": "likely downstream DB congestion",
	"suggested_actions": ["check DB latency", "increase timeout"],
	"related_cluster_ids": [],
	"confidence": 0.75
}`

// fakeProvider sets the canned response on the embedded fake.Provider and
// returns a type-asserted handle so tests can inspect calls.
func fakeProv(t *testing.T, ll *logstruct) *fake.Provider {
	t.Helper()
	fp, ok := ll.provider.(*fake.Provider)
	require.True(t, ok, "expected provider to be *fake.Provider")
	return fp
}

func TestAnalyzeNow_ReturnsAnalysis(t *testing.T) {
	ll, err := New(Config{
		Inline:  InlineConfig{Enabled: true},
		AI:      AIConfig{Provider: "fake"},
		Storage: StorageConfig{Kind: "memory"},
		Logger:  newTestLogger(t),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ll.Close() })

	fakeProv(t, ll).SetResponse(llm.Response{
		Content:      validAnalysisJSON,
		Model:        "fake-model",
		InputTokens:  10,
		OutputTokens: 20,
		LatencyMs:    5,
	})

	a, err := ll.AnalyzeNow(context.Background(), "payment service timeout for order 12345", Fields{
		"order_id": "12345",
		"service":  "payment",
	})
	require.NoError(t, err)
	require.NotNil(t, a)

	assert.Equal(t, "payment service is timing out", a.Summary)
	assert.NotEmpty(t, a.RootCauseHypothesis)
	assert.NotEmpty(t, a.SuggestedActions)
	assert.Equal(t, "", a.ClusterID, "AnalyzeNow must not assign a cluster ID")
	assert.False(t, a.CreatedAt.IsZero())
	assert.InDelta(t, 0.75, a.Confidence, 0.001)
}

func TestAnalyzeNow_EmptyMessageReturnsError(t *testing.T) {
	ll, err := New(Config{
		Inline:  InlineConfig{Enabled: true},
		AI:      AIConfig{Provider: "fake"},
		Storage: StorageConfig{Kind: "memory"},
		Logger:  newTestLogger(t),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ll.Close() })

	_, err = ll.AnalyzeNow(context.Background(), "   ", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty message")
}

func TestReport_DropCounterIncrements(t *testing.T) {
	ll, err := New(Config{
		Inline:  InlineConfig{Enabled: true, MinPriority: 100},
		AI:      AIConfig{Provider: "fake"},
		Storage: StorageConfig{Kind: "memory"},
		Logger:  newTestLogger(t),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ll.Close() })

	ctx := context.Background()
	require.NoError(t, ll.Start(ctx))

	// Flood the channel (rawChannelSize = 512) without the pipeline draining it.
	// We need to block the pipeline goroutine so events accumulate.
	// Easiest: close the run context so pipeline exits, then send a burst.
	ll.cancel()
	time.Sleep(20 * time.Millisecond) // let pipelineLoop exit

	// Now the consumer is gone — every Report goes to default branch.
	const burst = 600
	for i := 0; i < burst; i++ {
		ll.Report(ctx, errors.New("overflow"), nil)
	}

	dropped := ll.Stats().Dropped
	assert.Greater(t, dropped, int64(0), "expected some events to be dropped when pipeline is stopped")
}

// TestInlineAI_BackgroundAnalysisPersists is the key R7 integration test:
// events flow through the pipeline, a cluster reaches MinPriority = 0 so the
// worker pool submits it for analysis, and the result appears in
// RecentRecommendations.
func TestInlineAI_BackgroundAnalysisPersists(t *testing.T) {
	ll, err := New(Config{
		Inline:  InlineConfig{Enabled: true, MinPriority: -1}, // analyze all clusters; 0 is overridden to 50 by applyDefaults
		AI:      AIConfig{Provider: "fake"},
		Storage: StorageConfig{Kind: "memory"},
		Logger:  newTestLogger(t),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ll.Close() })

	// Give the fake a response that survives parseAnalysisJSON.
	fakeProv(t, ll).SetResponse(llm.Response{
		Content:      validAnalysisJSON,
		Model:        "fake-model",
		InputTokens:  10,
		OutputTokens: 20,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, ll.Start(ctx))

	// Report enough events to create and observe a cluster.
	for i := 0; i < 8; i++ {
		ll.Report(ctx, errors.New("db: connection pool exhausted"), Fields{"svc": "inventory"})
	}

	require.Eventually(t, func() bool {
		analyses, err := ll.RecentRecommendations(ctx, 10)
		return err == nil && len(analyses) > 0
	}, 5*time.Second, 50*time.Millisecond,
		"expected at least one persisted analysis via background worker")

	analyses, err := ll.RecentRecommendations(ctx, 10)
	require.NoError(t, err)
	require.NotEmpty(t, analyses)
	assert.Equal(t, "payment service is timing out", analyses[0].Summary)
	assert.NotEmpty(t, analyses[0].ClusterID, "persisted analysis must reference a cluster")
}

func TestClose_Idempotent(t *testing.T) {
	ll, err := New(Config{
		Inline:  InlineConfig{Enabled: true},
		AI:      AIConfig{Provider: "fake"},
		Storage: StorageConfig{Kind: "memory"},
		Logger:  newTestLogger(t),
	})
	require.NoError(t, err)

	require.NoError(t, ll.Start(context.Background()))
	require.NoError(t, ll.Close())
	require.NoError(t, ll.Close()) // second call must not panic or error
}
