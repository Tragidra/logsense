package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Tragidra/logsense/internal/analyze"
	"github.com/Tragidra/logsense/internal/api/dto"
	"github.com/Tragidra/logsense/internal/storage"
	"github.com/Tragidra/logsense/model"
)

type clustersHandler struct {
	repo     storage.Repository
	analyzer analyze.Analyzer
	logger   *slog.Logger
}

func newClustersHandler(repo storage.Repository, analyzer analyze.Analyzer, logger *slog.Logger) *clustersHandler {
	return &clustersHandler{repo: repo, analyzer: analyzer, logger: logger}
}

// GET /api/clusters
func (h *clustersHandler) list(w http.ResponseWriter, r *http.Request) {
	filter, err := parseClusterFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_filter", err.Error())
		return
	}
	clusters, total, err := h.repo.ListClusters(r.Context(), filter)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "list clusters", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list clusters")
		return
	}
	writeJSON(w, http.StatusOK, dto.ListClustersResponse{
		Items: toClusterDTOs(clusters),
		Total: total,
	})
}

// GET /api/clusters/{id}
func (h *clustersHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cluster, err := h.repo.GetCluster(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "cluster not found")
			return
		}
		h.logger.ErrorContext(r.Context(), "get cluster", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get cluster")
		return
	}
	writeJSON(w, http.StatusOK, toClusterDTO(cluster))
}

// GET /api/clusters/{id}/events
func (h *clustersHandler) events(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	q := r.URL.Query()

	filter := storage.EventFilter{Limit: 50}

	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_param", "from: "+err.Error())
			return
		}
		filter.From = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_param", "to: "+err.Error())
			return
		}
		filter.To = &t
	}
	if v := q.Get("level"); v != "" {
		l := model.ParseLevel(v)
		filter.Levels = []model.Level{l}
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 500 {
			writeError(w, http.StatusBadRequest, "invalid_param", "limit must be 1-500")
			return
		}
		filter.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid_param", "offset must be >= 0")
			return
		}
		filter.Offset = n
	}

	events, err := h.repo.ListEventsByCluster(r.Context(), id, filter)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "list events", "cluster_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list events")
		return
	}

	items := make([]dto.LogEventDTO, len(events))
	for i, e := range events {
		items[i] = dto.LogEventDTO{
			ID:        e.ID,
			Message:   e.Message,
			Level:     e.Level.String(),
			Service:   e.Service,
			Timestamp: e.Timestamp,
			Raw:       e.Raw,
		}
	}
	writeJSON(w, http.StatusOK, dto.ListEventsResponse{Items: items})
}

// POST /api/clusters/{id}/analyze
func (h *clustersHandler) triggerAnalyze(w http.ResponseWriter, r *http.Request) {
	if h.analyzer == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "analyzer not configured")
		return
	}
	id := chi.URLParam(r, "id")
	an, err := h.analyzer.Analyze(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "cluster not found")
			return
		}
		h.logger.ErrorContext(r.Context(), "trigger analyze", "cluster_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "analysis failed")
		return
	}
	writeJSON(w, http.StatusOK, dto.AnalyzeTriggerResponse{Analysis: toAnalysisDTO(an)})
}

func parseClusterFilter(r *http.Request) (storage.ClusterFilter, error) {
	q := r.URL.Query()
	f := storage.ClusterFilter{
		Limit:   50,
		OrderBy: "priority_desc",
	}

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 500 {
			return f, fmt.Errorf("limit must be 1-500")
		}
		f.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return f, fmt.Errorf("offset must be >= 0")
		}
		f.Offset = n
	}
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, fmt.Errorf("from: %w", err)
		}
		f.From = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, fmt.Errorf("to: %w", err)
		}
		f.To = &t
	}
	if v := q.Get("min_priority"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 100 {
			return f, fmt.Errorf("min_priority must be 0-100")
		}
		f.MinPriority = &n
	}
	if v := q.Get("services"); v != "" {
		f.Services = strings.Split(v, ",")
	}
	if v := q.Get("levels"); v != "" {
		for _, ls := range strings.Split(v, ",") {
			f.Levels = append(f.Levels, model.ParseLevel(strings.TrimSpace(ls)))
		}
	}
	if v := q.Get("search"); v != "" {
		f.SearchTemplate = v
	}
	if v := q.Get("order_by"); v != "" {
		switch v {
		case "priority_desc", "last_seen_desc", "count_desc":
			f.OrderBy = v
		default:
			return f, fmt.Errorf("order_by must be one of priority_desc, last_seen_desc, count_desc")
		}
	}
	return f, nil
}

func toClusterDTOs(clusters []model.Cluster) []dto.ClusterDTO {
	out := make([]dto.ClusterDTO, len(clusters))
	for i, c := range clusters {
		out[i] = toClusterDTO(c)
	}
	return out
}

func toClusterDTO(c model.Cluster) dto.ClusterDTO {
	levels := make(map[string]int64, len(c.Levels))
	for l, n := range c.Levels {
		levels[l.String()] = n
	}
	d := dto.ClusterDTO{
		ID:           c.ID,
		Template:     c.Template,
		Count:        c.Count,
		Priority:     c.Priority,
		AnomalyFlags: c.AnomalyFlags,
		Services:     c.Services,
		Levels:       levels,
		FirstSeen:    c.FirstSeen,
		LastSeen:     c.LastSeen,
		Examples:     c.ExamplesSample,
	}
	if c.LatestAnalysis != nil {
		d.Analysis = toAnalysisDTO(c.LatestAnalysis)
	}
	return d
}

func toAnalysisDTO(a *model.Analysis) *dto.AnalysisDTO {
	if a == nil {
		return nil
	}
	return &dto.AnalysisDTO{
		Summary:             a.Summary,
		Severity:            a.Severity.String(),
		RootCauseHypothesis: a.RootCauseHypothesis,
		SuggestedActions:    a.SuggestedActions,
		Confidence:          a.Confidence,
		ModelUsed:           a.ModelUsed,
		CreatedAt:           a.CreatedAt,
	}
}
