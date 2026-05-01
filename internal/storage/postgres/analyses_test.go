package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tragidra/logstruct/model"
)

func makeAnalysis(clusterID string, windowEnd time.Time) model.Analysis {
	return model.Analysis{
		ID:                  model.NewID(),
		ClusterID:           clusterID,
		WindowStart:         windowEnd.Add(-15 * time.Minute).UTC().Truncate(time.Millisecond),
		WindowEnd:           windowEnd.UTC().Truncate(time.Millisecond),
		Summary:             "Burst of auth failures from a single IP",
		Severity:            model.SeverityWarning,
		RootCauseHypothesis: "Credential stuffing against /login",
		SuggestedActions:    []string{"rate-limit /login", "block offending IP", "alert SOC"},
		RelatedClusterIDs:   []string{"cluster-related-1", "cluster-related-2"},
		Confidence:          0.82,
		ModelUsed:           "openrouter/anthropic/claude-3.5-sonnet",
		TokensInput:         1200,
		TokensOutput:        320,
		LatencyMs:           1450,
	}
}

func TestSaveAnalysis_RoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := seedCluster(t, store, ctx)
	a := makeAnalysis(c.ID, time.Now())

	require.NoError(t, store.SaveAnalysis(ctx, a))

	got, err := store.LatestAnalysisForCluster(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, a.ID, got.ID)
	assert.Equal(t, a.ClusterID, got.ClusterID)
	assert.WithinDuration(t, a.WindowStart, got.WindowStart, time.Millisecond)
	assert.WithinDuration(t, a.WindowEnd, got.WindowEnd, time.Millisecond)
	assert.Equal(t, a.Summary, got.Summary)
	assert.Equal(t, a.Severity, got.Severity)
	assert.Equal(t, a.RootCauseHypothesis, got.RootCauseHypothesis)
	assert.Equal(t, a.SuggestedActions, got.SuggestedActions)
	assert.Equal(t, a.RelatedClusterIDs, got.RelatedClusterIDs)
	assert.InDelta(t, a.Confidence, got.Confidence, 0.0001)
	assert.Equal(t, a.ModelUsed, got.ModelUsed)
	assert.Equal(t, a.TokensInput, got.TokensInput)
	assert.Equal(t, a.TokensOutput, got.TokensOutput)
	assert.Equal(t, a.LatencyMs, got.LatencyMs)
	assert.False(t, got.CreatedAt.IsZero(), "created_at should be set by the DB default")
}

func TestLatestAnalysisForCluster_NoRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := seedCluster(t, store, ctx)

	got, err := store.LatestAnalysisForCluster(ctx, c.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "no analysis → (nil, nil)")
}

func TestLatestAnalysisForCluster_PicksMostRecentWindow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := seedCluster(t, store, ctx)
	base := time.Now().UTC().Truncate(time.Millisecond)

	older := makeAnalysis(c.ID, base.Add(-2*time.Hour))
	newer := makeAnalysis(c.ID, base)

	require.NoError(t, store.SaveAnalysis(ctx, older))
	require.NoError(t, store.SaveAnalysis(ctx, newer))

	got, err := store.LatestAnalysisForCluster(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, newer.ID, got.ID)
}

func TestSaveAnalysis_UpsertSameWindow(t *testing.T) {
	// Re-running LLM analysis for the same window should update in place.
	store := newTestStore(t)
	ctx := context.Background()

	c := seedCluster(t, store, ctx)
	first := makeAnalysis(c.ID, time.Now())
	require.NoError(t, store.SaveAnalysis(ctx, first))

	updated := first
	updated.ID = model.NewID()
	updated.Summary = "Revised summary after re-run"
	updated.Severity = model.SeverityCritical
	updated.SuggestedActions = []string{"page on-call"}

	require.NoError(t, store.SaveAnalysis(ctx, updated))

	got, err := store.LatestAnalysisForCluster(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, first.ID, got.ID, "id stays with the original row")
	assert.Equal(t, updated.Summary, got.Summary)
	assert.Equal(t, updated.Severity, got.Severity)
	assert.Equal(t, updated.SuggestedActions, got.SuggestedActions)
}

func TestSaveAnalysis_EmptyRootCauseStoresNull(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := seedCluster(t, store, ctx)
	a := makeAnalysis(c.ID, time.Now())
	a.RootCauseHypothesis = ""

	require.NoError(t, store.SaveAnalysis(ctx, a))

	got, err := store.LatestAnalysisForCluster(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got.RootCauseHypothesis)
}

func TestSaveAnalysis_CascadesWithClusterDelete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c := seedCluster(t, store, ctx)
	a := makeAnalysis(c.ID, time.Now())
	require.NoError(t, store.SaveAnalysis(ctx, a))

	_, err := store.Pool().Exec(ctx, `DELETE FROM clusters WHERE id = $1`, c.ID)
	require.NoError(t, err)

	got, err := store.LatestAnalysisForCluster(ctx, c.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}
