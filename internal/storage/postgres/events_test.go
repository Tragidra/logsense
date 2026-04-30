package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tragidra/loglens/internal/storage"
	"github.com/Tragidra/loglens/model"
)

// seedCluster inserts a minimal cluster so events can reference it (app-side integrity).
func seedCluster(t *testing.T, store *Store, ctx context.Context) model.Cluster {
	t.Helper()
	c := makeCluster(model.NewID(), "fp-events-"+model.NewID())
	require.NoError(t, store.UpsertCluster(ctx, c))
	return c
}

func makeEvent(clusterID string, ts time.Time, level model.Level) model.LogEvent {
	return model.LogEvent{
		ID:        model.NewID(),
		Timestamp: ts.UTC().Truncate(time.Millisecond),
		Level:     level,
		Message:   "test log message",
		Service:   "test-svc",
		Source:    "file",
		Raw:       "raw line",
		Fields:    map[string]any{"key": "value", "count": 42},
	}
}

func TestSaveEvent_Basic(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := seedCluster(t, store, ctx)
	e := makeEvent(c.ID, time.Now(), model.LevelInfo)

	require.NoError(t, store.SaveEvent(ctx, e, c.ID))

	to := time.Now().Add(time.Minute)
	events, err := store.ListEventsByCluster(ctx, c.ID, storage.EventFilter{To: &to})
	require.NoError(t, err)
	require.Len(t, events, 1)

	got := events[0]
	assert.Equal(t, e.ID, got.ID)
	assert.Equal(t, e.Level, got.Level)
	assert.Equal(t, e.Message, got.Message)
	assert.Equal(t, e.Service, got.Service)
	assert.Equal(t, e.Source, got.Source)
	assert.Equal(t, e.Raw, got.Raw)
	assert.Equal(t, e.TraceID, got.TraceID)
	assert.WithinDuration(t, e.Timestamp, got.Timestamp, time.Millisecond)
	assert.Equal(t, "value", got.Fields["key"])
}

func TestSaveEvent_AcrossDays(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := seedCluster(t, store, ctx)

	day1 := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)

	e1 := makeEvent(c.ID, day1, model.LevelError)
	e2 := makeEvent(c.ID, day2, model.LevelWarn)

	require.NoError(t, store.SaveEvent(ctx, e1, c.ID))
	require.NoError(t, store.SaveEvent(ctx, e2, c.ID))

	from := day1.Add(-time.Hour)
	to := day2.Add(time.Hour)
	events, err := store.ListEventsByCluster(ctx, c.ID, storage.EventFilter{From: &from, To: &to})
	require.NoError(t, err)
	require.Len(t, events, 2)

	ids := []string{events[0].ID, events[1].ID}
	assert.Contains(t, ids, e1.ID)
	assert.Contains(t, ids, e2.ID)
}

func TestListEventsByCluster_OrderedByTsDesc(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := seedCluster(t, store, ctx)
	base := time.Now().UTC().Truncate(time.Second)

	for i := 0; i < 3; i++ {
		e := makeEvent(c.ID, base.Add(time.Duration(i)*time.Second), model.LevelInfo)
		require.NoError(t, store.SaveEvent(ctx, e, c.ID))
	}

	to := base.Add(time.Minute)
	from := base.Add(-time.Minute)
	events, err := store.ListEventsByCluster(ctx, c.ID, storage.EventFilter{From: &from, To: &to})
	require.NoError(t, err)
	require.Len(t, events, 3)

	assert.True(t, events[0].Timestamp.After(events[1].Timestamp) || events[0].Timestamp.Equal(events[1].Timestamp))
	assert.True(t, events[1].Timestamp.After(events[2].Timestamp) || events[1].Timestamp.Equal(events[2].Timestamp))
}

func TestListEventsByCluster_FilterLevel(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := seedCluster(t, store, ctx)
	base := time.Now().UTC()

	for _, lvl := range []model.Level{model.LevelInfo, model.LevelError, model.LevelWarn} {
		e := makeEvent(c.ID, base, lvl)
		require.NoError(t, store.SaveEvent(ctx, e, c.ID))
	}

	to := base.Add(time.Minute)
	from := base.Add(-time.Minute)
	events, err := store.ListEventsByCluster(ctx, c.ID, storage.EventFilter{
		From:   &from,
		To:     &to,
		Levels: []model.Level{model.LevelError},
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, model.LevelError, events[0].Level)
}

func TestListEventsByCluster_Pagination(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := seedCluster(t, store, ctx)
	base := time.Now().UTC()

	for i := 0; i < 5; i++ {
		e := makeEvent(c.ID, base.Add(time.Duration(i)*time.Second), model.LevelInfo)
		require.NoError(t, store.SaveEvent(ctx, e, c.ID))
	}

	from := base.Add(-time.Minute)
	to := base.Add(time.Minute)

	page1, err := store.ListEventsByCluster(ctx, c.ID, storage.EventFilter{From: &from, To: &to, Limit: 2, Offset: 0})
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	page2, err := store.ListEventsByCluster(ctx, c.ID, storage.EventFilter{From: &from, To: &to, Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Len(t, page2, 2)

	assert.NotEqual(t, page1[0].ID, page2[0].ID)
}

func TestListEventsByCluster_DefaultTimeRange(t *testing.T) {
	// Without explicit From, the default is last 1 hour and verify old events are excluded
	store := newTestStore(t)
	ctx := context.Background()

	c := seedCluster(t, store, ctx)

	old := makeEvent(c.ID, time.Now().UTC().Add(-2*time.Hour), model.LevelInfo)
	recent := makeEvent(c.ID, time.Now().UTC().Add(-30*time.Minute), model.LevelInfo)

	require.NoError(t, store.SaveEvent(ctx, old, c.ID))
	require.NoError(t, store.SaveEvent(ctx, recent, c.ID))

	to := time.Now().Add(time.Minute)
	events, err := store.ListEventsByCluster(ctx, c.ID, storage.EventFilter{To: &to})
	require.NoError(t, err)

	// Default from = now - 1h, but the -2h event should be excluded
	for _, e := range events {
		assert.NotEqual(t, old.ID, e.ID, "old event should be excluded by default time range")
	}
	ids := make([]string, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	assert.Contains(t, ids, recent.ID)
}
