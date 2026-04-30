package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tragidra/logsense/internal/storage"
	"github.com/Tragidra/logsense/model"
)

// makeCluster builds a minimal valid Cluster for tests.
func makeCluster(id, fingerprint string) model.Cluster {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return model.Cluster{
		ID:             id,
		Fingerprint:    fingerprint,
		Template:       "user <*> failed login from <*>",
		TemplateTokens: []string{"user", "<*>", "failed", "login", "from", "<*>"},
		Services:       []string{"auth"},
		Levels:         map[model.Level]int64{model.LevelError: 5, model.LevelWarn: 2},
		Count:          7,
		FirstSeen:      now.Add(-1 * time.Hour),
		LastSeen:       now,
		ExamplesSample: []string{"user alice failed login from 1.2.3.4"},
		Priority:       60,
		AnomalyFlags:   []string{"burst"},
	}
}

func TestUpsertCluster_Insert(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := makeCluster(model.NewID(), "fp-insert-01")
	require.NoError(t, store.UpsertCluster(ctx, c))

	got, err := store.GetCluster(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, c.ID, got.ID)
	assert.Equal(t, c.Fingerprint, got.Fingerprint)
	assert.Equal(t, c.Template, got.Template)
	assert.Equal(t, c.Count, got.Count)
	assert.Equal(t, c.Priority, got.Priority)
	assert.Equal(t, c.Services, got.Services)
	assert.Equal(t, c.AnomalyFlags, got.AnomalyFlags)
	assert.Equal(t, c.ExamplesSample, got.ExamplesSample)
	assert.Equal(t, c.Levels, got.Levels)
	assert.WithinDuration(t, c.FirstSeen, got.FirstSeen, time.Millisecond)
	assert.WithinDuration(t, c.LastSeen, got.LastSeen, time.Millisecond)
}

func TestUpsertCluster_Update(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := makeCluster(model.NewID(), "fp-update-01")
	require.NoError(t, store.UpsertCluster(ctx, c))

	// Simulate a new burst
	c2 := c
	c2.Count = 20
	c2.Priority = 80
	c2.LastSeen = c.LastSeen.Add(5 * time.Minute)
	c2.AnomalyFlags = []string{"burst", "novel"}
	require.NoError(t, store.UpsertCluster(ctx, c2))

	got, err := store.GetCluster(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(20), got.Count)
	assert.Equal(t, 80, got.Priority)
	assert.ElementsMatch(t, []string{"burst", "novel"}, got.AnomalyFlags)
}

// TestUpsertCluster_FingerprintCanChange covers the case where Drain generalizes a cluster's template, the fingerprint
// is recomputed but the cluster ID stays stable. The upsert must update the row keyed by id, not
// reject it on a fingerprint conflict.
func TestUpsertCluster_FingerprintCanChange(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := makeCluster(model.NewID(), "fp-original")
	c.Template = "user 42 logged in"
	require.NoError(t, store.UpsertCluster(ctx, c))

	c.Fingerprint = "fp-generalized"
	c.Template = "user <*> logged in"
	c.Count = 5
	require.NoError(t, store.UpsertCluster(ctx, c))

	got, err := store.GetCluster(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "fp-generalized", got.Fingerprint)
	assert.Equal(t, "user <*> logged in", got.Template)
	assert.Equal(t, int64(5), got.Count)
}

func TestGetCluster_NotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetCluster(context.Background(), "nonexistent-id")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestListClusters_Basic(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i, fp := range []string{"fp-list-01", "fp-list-02", "fp-list-03"} {
		c := makeCluster(model.NewID(), fp)
		c.Priority = 10 * (i + 1)
		require.NoError(t, store.UpsertCluster(ctx, c))
	}

	clusters, total, err := store.ListClusters(ctx, storage.ClusterFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, clusters, 3)
}

func TestListClusters_FilterMinPriority(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for _, prio := range []int{10, 40, 70} {
		c := makeCluster(model.NewID(), "fp-prio-"+model.NewID())
		c.Priority = prio
		require.NoError(t, store.UpsertCluster(ctx, c))
	}

	minPrio := 40
	clusters, total, err := store.ListClusters(ctx, storage.ClusterFilter{MinPriority: &minPrio})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	for _, c := range clusters {
		assert.GreaterOrEqual(t, c.Priority, 40)
	}
}

func TestListClusters_FilterServices(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c1 := makeCluster(model.NewID(), "fp-svc-01")
	c1.Services = []string{"auth", "api"}
	require.NoError(t, store.UpsertCluster(ctx, c1))

	c2 := makeCluster(model.NewID(), "fp-svc-02")
	c2.Services = []string{"worker"}
	require.NoError(t, store.UpsertCluster(ctx, c2))

	clusters, _, err := store.ListClusters(ctx, storage.ClusterFilter{Services: []string{"auth"}})
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	assert.Equal(t, c1.ID, clusters[0].ID)
}

func TestListClusters_FilterTimeRange(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	old := makeCluster(model.NewID(), "fp-time-old")
	old.LastSeen = now.Add(-48 * time.Hour)
	old.FirstSeen = old.LastSeen.Add(-1 * time.Hour)
	require.NoError(t, store.UpsertCluster(ctx, old))

	recent := makeCluster(model.NewID(), "fp-time-recent")
	recent.LastSeen = now.Add(-1 * time.Hour)
	recent.FirstSeen = recent.LastSeen.Add(-30 * time.Minute)
	require.NoError(t, store.UpsertCluster(ctx, recent))

	from := now.Add(-6 * time.Hour)
	clusters, total, err := store.ListClusters(ctx, storage.ClusterFilter{From: &from})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, recent.ID, clusters[0].ID)
}

func TestListClusters_FilterLevels(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	cFatal := makeCluster(model.NewID(), "fp-lvl-fatal")
	cFatal.Levels = map[model.Level]int64{model.LevelFatal: 3}
	require.NoError(t, store.UpsertCluster(ctx, cFatal))

	cInfo := makeCluster(model.NewID(), "fp-lvl-info")
	cInfo.Levels = map[model.Level]int64{model.LevelInfo: 100}
	require.NoError(t, store.UpsertCluster(ctx, cInfo))

	clusters, _, err := store.ListClusters(ctx, storage.ClusterFilter{
		Levels: []model.Level{model.LevelFatal},
	})
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	assert.Equal(t, cFatal.ID, clusters[0].ID)
}

func TestListClusters_FilterSearchTemplate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c1 := makeCluster(model.NewID(), "fp-search-01")
	c1.Template = "database connection timeout"
	require.NoError(t, store.UpsertCluster(ctx, c1))

	c2 := makeCluster(model.NewID(), "fp-search-02")
	c2.Template = "user login failed"
	require.NoError(t, store.UpsertCluster(ctx, c2))

	clusters, total, err := store.ListClusters(ctx, storage.ClusterFilter{SearchTemplate: "timeout"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, c1.ID, clusters[0].ID)
}

func TestListClusters_OrderBy(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	priorities := []int{30, 70, 50}
	for i, prio := range priorities {
		c := makeCluster(model.NewID(), "fp-order-"+model.NewID())
		c.Priority = prio
		c.Count = int64(i + 1)
		require.NoError(t, store.UpsertCluster(ctx, c))
	}

	t.Run("priority_desc", func(t *testing.T) {
		clusters, _, err := store.ListClusters(ctx, storage.ClusterFilter{OrderBy: "priority_desc"})
		require.NoError(t, err)
		require.Len(t, clusters, 3)
		assert.Equal(t, 70, clusters[0].Priority)
		assert.Equal(t, 30, clusters[2].Priority)
	})

	t.Run("count_desc", func(t *testing.T) {
		clusters, _, err := store.ListClusters(ctx, storage.ClusterFilter{OrderBy: "count_desc"})
		require.NoError(t, err)
		require.Len(t, clusters, 3)
		assert.GreaterOrEqual(t, clusters[0].Count, clusters[1].Count)
	})
}

func TestListClusters_Pagination(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		c := makeCluster(model.NewID(), "fp-page-"+model.NewID())
		require.NoError(t, store.UpsertCluster(ctx, c))
	}

	page1, total, err := store.ListClusters(ctx, storage.ClusterFilter{Limit: 2, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, page1, 2)

	page2, _, err := store.ListClusters(ctx, storage.ClusterFilter{Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Len(t, page2, 2)

	// Pages must be distinct.
	assert.NotEqual(t, page1[0].ID, page2[0].ID)
}

func TestPruneStaleClusters(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	old := makeCluster(model.NewID(), "fp-prune-old")
	old.LastSeen = now.Add(-100 * time.Hour)
	old.FirstSeen = old.LastSeen.Add(-1 * time.Hour)
	require.NoError(t, store.UpsertCluster(ctx, old))

	fresh := makeCluster(model.NewID(), "fp-prune-fresh")
	fresh.LastSeen = now.Add(-1 * time.Hour)
	fresh.FirstSeen = fresh.LastSeen.Add(-30 * time.Minute)
	require.NoError(t, store.UpsertCluster(ctx, fresh))

	deleted, err := store.PruneStaleClusters(ctx, now.Add(-72*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	_, err = store.GetCluster(ctx, old.ID)
	assert.ErrorIs(t, err, storage.ErrNotFound)

	_, err = store.GetCluster(ctx, fresh.ID)
	assert.NoError(t, err)
}
