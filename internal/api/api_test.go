package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tragidra/logsense/internal/analyze"
	"github.com/Tragidra/logsense/internal/api"
	"github.com/Tragidra/logsense/internal/api/dto"
	"github.com/Tragidra/logsense/internal/config"
	"github.com/Tragidra/logsense/internal/storage"
	"github.com/Tragidra/logsense/model"

	"log/slog"
)

func TestHealthz(t *testing.T) {
	srv := newTestServer(t, newFakeRepo(), nil)
	resp := do(t, srv, "GET", "/api/healthz", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := decodeMap(t, resp)
	assert.Equal(t, "ok", body["status"])
}

func TestReadyz_OK(t *testing.T) {
	srv := newTestServer(t, newFakeRepo(), nil)
	resp := do(t, srv, "GET", "/api/readyz", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestReadyz_StorageDown(t *testing.T) {
	repo := newFakeRepo()
	repo.pingErr = errors.New("db down")
	srv := newTestServer(t, repo, nil)
	resp := do(t, srv, "GET", "/api/readyz", nil)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

// clusters list
func TestListClusters_Empty(t *testing.T) {
	srv := newTestServer(t, newFakeRepo(), nil)
	resp := do(t, srv, "GET", "/api/clusters", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body dto.ListClustersResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Empty(t, body.Items)
	assert.Equal(t, int64(0), body.Total)
}

func TestListClusters_ReturnsClusters(t *testing.T) {
	repo := newFakeRepo()
	repo.clusters = []model.Cluster{
		makeCluster("id-1", "template A <*>", 75),
		makeCluster("id-2", "template B <*>", 50),
	}
	srv := newTestServer(t, repo, nil)
	resp := do(t, srv, "GET", "/api/clusters", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body dto.ListClustersResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Len(t, body.Items, 2)
	assert.Equal(t, "id-1", body.Items[0].ID)
	assert.Equal(t, int64(2), body.Total)
}

func TestListClusters_InvalidLimit(t *testing.T) {
	srv := newTestServer(t, newFakeRepo(), nil)
	resp := do(t, srv, "GET", "/api/clusters?limit=0", nil)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestListClusters_InvalidOrderBy(t *testing.T) {
	srv := newTestServer(t, newFakeRepo(), nil)
	resp := do(t, srv, "GET", "/api/clusters?order_by=bogus", nil)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// get cluster
func TestGetCluster_Found(t *testing.T) {
	repo := newFakeRepo()
	repo.clusters = []model.Cluster{makeCluster("abc123", "error <*> crashed", 90)}
	srv := newTestServer(t, repo, nil)

	resp := do(t, srv, "GET", "/api/clusters/abc123", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body dto.ClusterDTO
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "abc123", body.ID)
	assert.Equal(t, 90, body.Priority)
}

func TestGetCluster_NotFound(t *testing.T) {
	srv := newTestServer(t, newFakeRepo(), nil)
	resp := do(t, srv, "GET", "/api/clusters/missing", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetEvents_OK(t *testing.T) {
	repo := newFakeRepo()
	repo.events["cluster-1"] = []model.LogEvent{
		{ID: "e1", Message: "err msg", Level: model.LevelError, Timestamp: time.Now()},
	}
	srv := newTestServer(t, repo, nil)

	resp := do(t, srv, "GET", "/api/clusters/cluster-1/events", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body dto.ListEventsResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Items, 1)
	assert.Equal(t, "e1", body.Items[0].ID)
	assert.Equal(t, "error", body.Items[0].Level)
}

func TestGetEvents_InvalidLimit(t *testing.T) {
	srv := newTestServer(t, newFakeRepo(), nil)
	resp := do(t, srv, "GET", "/api/clusters/x/events?limit=99999", nil)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// run analyze
func TestTriggerAnalyze_OK(t *testing.T) {
	repo := newFakeRepo()
	repo.clusters = []model.Cluster{makeCluster("c1", "tmpl", 80)}

	an := &fakeAnalyzer{result: &model.Analysis{
		Summary:          "Everything looks fine.",
		Severity:         model.SeverityInfo,
		SuggestedActions: []string{"No action needed."},
		Confidence:       0.9,
		CreatedAt:        time.Now(),
	}}
	srv := newTestServer(t, repo, an)

	resp := do(t, srv, "POST", "/api/clusters/c1/analyze", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body dto.AnalyzeTriggerResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.NotNil(t, body.Analysis)
	assert.Equal(t, "Everything looks fine.", body.Analysis.Summary)
	assert.Equal(t, "info", body.Analysis.Severity)
}

func TestTriggerAnalyze_NotFound(t *testing.T) {
	an := &fakeAnalyzer{err: storage.ErrNotFound}
	srv := newTestServer(t, newFakeRepo(), an)
	resp := do(t, srv, "POST", "/api/clusters/missing/analyze", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestCORS_AllowedOrigin(t *testing.T) {
	srv := newTestServerWithCORS(t, newFakeRepo(), nil, []string{"http://localhost:5173"})
	req := httptest.NewRequest("GET", "/api/healthz", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	assert.Equal(t, "http://localhost:5173", rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	srv := newTestServerWithCORS(t, newFakeRepo(), nil, []string{"http://localhost:5173"})
	req := httptest.NewRequest("GET", "/api/healthz", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

func newTestServer(t *testing.T, repo *fakeRepo, an analyze.Analyzer) *httptest.Server {
	t.Helper()
	s := api.New(config.APIConfig{}, repo, an, slog.Default())
	return httptest.NewServer(s.Handler())
}

func newTestServerWithCORS(t *testing.T, repo *fakeRepo, an analyze.Analyzer, origins []string) http.Handler {
	t.Helper()
	s := api.New(config.APIConfig{AllowedOrigins: origins}, repo, an, slog.Default())
	return s.Handler()
}

func do(t *testing.T, srv *httptest.Server, method, path string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, body)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decodeMap(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	return m
}

func makeCluster(id, tmpl string, priority int) model.Cluster {
	return model.Cluster{
		ID:           id,
		Template:     tmpl,
		Priority:     priority,
		Services:     []string{"svc"},
		Levels:       map[model.Level]int64{model.LevelError: 1},
		Count:        1,
		FirstSeen:    time.Now().Add(-time.Hour),
		LastSeen:     time.Now(),
		AnomalyFlags: []string{},
	}
}

type fakeRepo struct {
	mu       sync.Mutex
	clusters []model.Cluster
	events   map[string][]model.LogEvent
	pingErr  error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{events: make(map[string][]model.LogEvent)}
}

func (f *fakeRepo) GetCluster(_ context.Context, id string) (model.Cluster, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.clusters {
		if c.ID == id {
			return c, nil
		}
	}
	return model.Cluster{}, storage.ErrNotFound
}

func (f *fakeRepo) ListClusters(_ context.Context, _ storage.ClusterFilter) ([]model.Cluster, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.clusters, int64(len(f.clusters)), nil
}

func (f *fakeRepo) ListEventsByCluster(_ context.Context, clusterID string, _ storage.EventFilter) ([]model.LogEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.events[clusterID], nil
}

func (f *fakeRepo) Ping(_ context.Context) error { return f.pingErr }

func (f *fakeRepo) UpsertCluster(_ context.Context, _ model.Cluster) error { return nil }
func (f *fakeRepo) PruneStaleClusters(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeRepo) SaveEvent(_ context.Context, _ model.LogEvent, _ string) error { return nil }
func (f *fakeRepo) SaveAnalysis(_ context.Context, _ model.Analysis) error        { return nil }
func (f *fakeRepo) LatestAnalysisForCluster(_ context.Context, _ string) (*model.Analysis, error) {
	return nil, nil
}
func (f *fakeRepo) ListRecentAnalyses(_ context.Context, _ int) ([]model.Analysis, error) {
	return nil, nil
}
func (f *fakeRepo) Close() error { return nil }

type fakeAnalyzer struct {
	result *model.Analysis
	err    error
}

func (a *fakeAnalyzer) Analyze(_ context.Context, _ string) (*model.Analysis, error) {
	return a.result, a.err
}
