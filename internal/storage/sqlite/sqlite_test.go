package sqlite

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tragidra/loglens/internal/storage"
	"github.com/Tragidra/loglens/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	s, err := New(context.Background(), Config{Path: path}, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestNew_AppliesMigrations(t *testing.T) {
	s := newTestStore(t)

	rows, err := s.db.Query(`SELECT version FROM loglens_migrations ORDER BY version`)
	require.NoError(t, err)
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var v string
		require.NoError(t, rows.Scan(&v))
		versions = append(versions, v)
	}
	assert.Equal(t, []string{"001", "002", "003", "004"}, versions)
}

func TestNew_MigrationsAreIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	s1, err := New(context.Background(), Config{Path: path}, logger)
	require.NoError(t, err)
	require.NoError(t, s1.Close())

	s2, err := New(context.Background(), Config{Path: path}, logger)
	require.NoError(t, err)
	require.NoError(t, s2.Close())
}

func TestUpsertAndGetCluster(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	c := model.Cluster{
		ID:             "cluster-1",
		Fingerprint:    "fp-1",
		Template:       "user <*> failed login from <*>",
		FirstSeen:      now.Add(-time.Hour),
		LastSeen:       now,
		Count:          42,
		Priority:       73,
		AnomalyFlags:   []string{"burst"},
		Services:       []string{"auth-svc"},
		Levels:         map[model.Level]int64{model.LevelError: 30, model.LevelWarn: 12},
		ExamplesSample: []string{"line a", "line b"},
	}
	require.NoError(t, s.UpsertCluster(ctx, c))

	got, err := s.GetCluster(ctx, "cluster-1")
	require.NoError(t, err)
	assert.Equal(t, c.ID, got.ID)
	assert.Equal(t, c.Fingerprint, got.Fingerprint)
	assert.Equal(t, c.Template, got.Template)
	assert.Equal(t, c.Count, got.Count)
	assert.Equal(t, c.Priority, got.Priority)
	assert.Equal(t, c.AnomalyFlags, got.AnomalyFlags)
	assert.Equal(t, c.Services, got.Services)
	assert.Equal(t, c.ExamplesSample, got.ExamplesSample)
	assert.Equal(t, c.Levels, got.Levels)
	assert.WithinDuration(t, c.FirstSeen, got.FirstSeen, time.Second)
	assert.WithinDuration(t, c.LastSeen, got.LastSeen, time.Second)
}

func TestGetCluster_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetCluster(context.Background(), "nope")
	assert.True(t, errors.Is(err, storage.ErrNotFound))
}

func TestUpsertCluster_OnConflictUpdatesMutableFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	c := model.Cluster{
		ID:          "c1",
		Fingerprint: "fp-shared",
		Template:    "tmpl",
		FirstSeen:   now.Add(-time.Hour),
		LastSeen:    now.Add(-30 * time.Minute),
		Count:       5,
		Priority:    20,
	}
	require.NoError(t, s.UpsertCluster(ctx, c))

	c.LastSeen = now
	c.Count = 50
	c.Priority = 90
	require.NoError(t, s.UpsertCluster(ctx, c))

	got, err := s.GetCluster(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, int64(50), got.Count)
	assert.Equal(t, 90, got.Priority)
	assert.WithinDuration(t, now, got.LastSeen, time.Second)
}

// TestUpsertCluster_FingerprintCanChange covers the case where Drain generalizes a cluster's template -
// the fingerprint is recomputed but the cluster ID stays stable. The upsert must update the row keyed by id, not
// reject it on a fingerprint conflict.
func TestUpsertCluster_FingerprintCanChange(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	c := model.Cluster{
		ID:          "cluster-stable",
		Fingerprint: "fp-original",
		Template:    "user 42 logged in",
		FirstSeen:   now.Add(-time.Hour),
		LastSeen:    now.Add(-30 * time.Minute),
		Count:       1,
	}
	require.NoError(t, s.UpsertCluster(ctx, c))

	c.Fingerprint = "fp-generalized"
	c.Template = "user <*> logged in"
	c.LastSeen = now
	c.Count = 5
	require.NoError(t, s.UpsertCluster(ctx, c))

	got, err := s.GetCluster(ctx, "cluster-stable")
	require.NoError(t, err)
	assert.Equal(t, "fp-generalized", got.Fingerprint)
	assert.Equal(t, "user <*> logged in", got.Template)
	assert.Equal(t, int64(5), got.Count)
}

func TestUpsertCluster_NilSlicesAreCoerced(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	c := model.Cluster{
		ID:          "c-nil",
		Fingerprint: "fp-nil",
		Template:    "tmpl",
		FirstSeen:   time.Now().UTC(),
		LastSeen:    time.Now().UTC(),
		// AnomalyFlags / Services / ExamplesSample left nil intentionally
	}
	require.NoError(t, s.UpsertCluster(ctx, c))

	got, err := s.GetCluster(ctx, "c-nil")
	require.NoError(t, err)
	assert.Empty(t, got.AnomalyFlags)
	assert.Empty(t, got.Services)
	assert.Empty(t, got.ExamplesSample)
}

func TestListClusters_FiltersAndOrders(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for i, prio := range []int{10, 70, 40, 90} {
		c := model.Cluster{
			ID:          "c" + string(rune('a'+i)),
			Fingerprint: "fp" + string(rune('a'+i)),
			Template:    "t",
			FirstSeen:   now,
			LastSeen:    now,
			Priority:    prio,
			Services:    []string{"svc-" + string(rune('a'+i))},
		}
		require.NoError(t, s.UpsertCluster(ctx, c))
	}

	min := 50
	out, total, err := s.ListClusters(ctx, storage.ClusterFilter{MinPriority: &min, OrderBy: "priority_desc"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, out, 2)
	assert.Equal(t, 90, out[0].Priority)
	assert.Equal(t, 70, out[1].Priority)
}

func TestListClusters_ServicesFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, s.UpsertCluster(ctx, model.Cluster{
		ID: "c1", Fingerprint: "fp1", Template: "t", FirstSeen: now, LastSeen: now,
		Services: []string{"auth", "api"},
	}))
	require.NoError(t, s.UpsertCluster(ctx, model.Cluster{
		ID: "c2", Fingerprint: "fp2", Template: "t", FirstSeen: now, LastSeen: now,
		Services: []string{"db"},
	}))

	out, _, err := s.ListClusters(ctx, storage.ClusterFilter{Services: []string{"auth"}})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "c1", out[0].ID)
}

func TestPruneStaleClusters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, s.UpsertCluster(ctx, model.Cluster{
		ID: "old", Fingerprint: "fp-old", Template: "t",
		FirstSeen: now.Add(-72 * time.Hour), LastSeen: now.Add(-48 * time.Hour),
	}))
	require.NoError(t, s.UpsertCluster(ctx, model.Cluster{
		ID: "fresh", Fingerprint: "fp-fresh", Template: "t",
		FirstSeen: now.Add(-time.Hour), LastSeen: now,
	}))

	n, err := s.PruneStaleClusters(ctx, now.Add(-24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	_, err = s.GetCluster(ctx, "old")
	assert.True(t, errors.Is(err, storage.ErrNotFound))

	_, err = s.GetCluster(ctx, "fresh")
	assert.NoError(t, err)
}

func TestSaveAndListEvents(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, s.UpsertCluster(ctx, model.Cluster{
		ID: "cl1", Fingerprint: "fp-cl1", Template: "t",
		FirstSeen: now.Add(-time.Hour), LastSeen: now,
	}))

	for i := 0; i < 3; i++ {
		require.NoError(t, s.SaveEvent(ctx, model.LogEvent{
			ID:        "evt-" + string(rune('a'+i)),
			Timestamp: now.Add(time.Duration(-i) * time.Minute),
			Level:     model.LevelError,
			Service:   "svc",
			Message:   "boom",
			Source:    "file",
			Fields:    map[string]any{"order_id": i},
		}, "cl1"))
	}

	from := now.Add(-time.Hour)
	events, err := s.ListEventsByCluster(ctx, "cl1", storage.EventFilter{From: &from})
	require.NoError(t, err)
	require.Len(t, events, 3)
	// ordered ts DESC
	assert.True(t, events[0].Timestamp.After(events[1].Timestamp))
	assert.NotNil(t, events[0].Fields)
}

func TestSaveAnalysis_AndListRecent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, s.UpsertCluster(ctx, model.Cluster{
		ID: "cl1", Fingerprint: "fp-cl1", Template: "t",
		FirstSeen: now.Add(-time.Hour), LastSeen: now,
	}))

	for i := 0; i < 3; i++ {
		a := model.Analysis{
			ID:                  "an-" + string(rune('a'+i)),
			ClusterID:           "cl1",
			WindowStart:         now.Add(time.Duration(-i-1) * time.Minute),
			WindowEnd:           now.Add(time.Duration(-i) * time.Minute),
			Summary:             "s",
			Severity:            model.SeverityWarning,
			RootCauseHypothesis: "rc",
			SuggestedActions:    []string{"action1"},
			Confidence:          0.5,
			ModelUsed:           "fake",
			CreatedAt:           now.Add(time.Duration(-i) * time.Second),
		}
		require.NoError(t, s.SaveAnalysis(ctx, a))
	}

	latest, err := s.LatestAnalysisForCluster(ctx, "cl1")
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, "an-a", latest.ID)

	recent, err := s.ListRecentAnalyses(ctx, 10)
	require.NoError(t, err)
	require.Len(t, recent, 3)
	// newest CreatedAt first
	assert.Equal(t, "an-a", recent[0].ID)
}

func TestSaveAnalysis_OnConflictUpdates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, s.UpsertCluster(ctx, model.Cluster{
		ID: "cl1", Fingerprint: "fp-cl1", Template: "t",
		FirstSeen: now, LastSeen: now,
	}))

	a := model.Analysis{
		ID:          "an-x",
		ClusterID:   "cl1",
		WindowStart: now.Add(-time.Minute),
		WindowEnd:   now,
		Summary:     "first",
		Severity:    model.SeverityInfo,
		ModelUsed:   "fake",
	}
	require.NoError(t, s.SaveAnalysis(ctx, a))

	a.Summary = "second"
	a.Severity = model.SeverityCritical
	require.NoError(t, s.SaveAnalysis(ctx, a))

	got, err := s.LatestAnalysisForCluster(ctx, "cl1")
	require.NoError(t, err)
	assert.Equal(t, "second", got.Summary)
	assert.Equal(t, model.SeverityCritical, got.Severity)
}

func TestSaveAnalysis_NilSlicesCoerced(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, s.UpsertCluster(ctx, model.Cluster{
		ID: "cl1", Fingerprint: "fp-cl1", Template: "t",
		FirstSeen: now, LastSeen: now,
	}))

	a := model.Analysis{
		ID:          "an-nil",
		ClusterID:   "cl1",
		WindowStart: now.Add(-time.Minute),
		WindowEnd:   now,
		Summary:     "s",
		Severity:    model.SeverityInfo,
		ModelUsed:   "fake",
		// SuggestedActions / RelatedClusterIDs intentionally nil
	}
	require.NoError(t, s.SaveAnalysis(ctx, a))

	got, err := s.LatestAnalysisForCluster(ctx, "cl1")
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got.SuggestedActions)
	assert.Empty(t, got.RelatedClusterIDs)
}
