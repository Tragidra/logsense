// Package memory provides an in-memory storage.Repository implementation.
//
// It is intended for tests and the loglens smoke test. Not durable — data is
// lost when the process exits. Not optimised for large volumes.
package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Tragidra/loglens/internal/storage"
	"github.com/Tragidra/loglens/model"
)

// Store is a goroutine-safe in-memory storage.Repository.
type Store struct {
	mu       sync.RWMutex
	clusters map[string]model.Cluster
	events   map[string][]model.LogEvent // clusterID -> events
	analyses []model.Analysis            // append order, latest at end
}

// New returns an empty in-memory store.
func New() *Store {
	return &Store{
		clusters: make(map[string]model.Cluster),
		events:   make(map[string][]model.LogEvent),
	}
}

func (s *Store) Ping(_ context.Context) error { return nil }
func (s *Store) Close() error                 { return nil }

func (s *Store) SaveEvent(_ context.Context, e model.LogEvent, clusterID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[clusterID] = append(s.events[clusterID], e)
	return nil
}

func (s *Store) ListEventsByCluster(_ context.Context, clusterID string, filter storage.EventFilter) ([]model.LogEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	src := s.events[clusterID]
	out := make([]model.LogEvent, 0, len(src))
	for _, e := range src {
		if filter.From != nil && e.Timestamp.Before(*filter.From) {
			continue
		}
		if filter.To != nil && e.Timestamp.After(*filter.To) {
			continue
		}
		if len(filter.Levels) > 0 && !containsLevel(filter.Levels, e.Level) {
			continue
		}
		out = append(out, e)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.After(out[j].Timestamp) })

	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	if filter.Offset >= len(out) {
		return nil, nil
	}
	out = out[filter.Offset:]
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func containsLevel(levels []model.Level, l model.Level) bool {
	for _, x := range levels {
		if x == l {
			return true
		}
	}
	return false
}

func (s *Store) UpsertCluster(_ context.Context, c model.Cluster) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.AnomalyFlags == nil {
		c.AnomalyFlags = []string{}
	}
	if c.Services == nil {
		c.Services = []string{}
	}
	if c.ExamplesSample == nil {
		c.ExamplesSample = []string{}
	}
	s.clusters[c.ID] = c
	return nil
}

func (s *Store) GetCluster(_ context.Context, id string) (model.Cluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.clusters[id]
	if !ok {
		return model.Cluster{}, storage.ErrNotFound
	}
	return c, nil
}

func (s *Store) ListClusters(_ context.Context, filter storage.ClusterFilter) ([]model.Cluster, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]model.Cluster, 0, len(s.clusters))
	for _, c := range s.clusters {
		if filter.From != nil && c.LastSeen.Before(*filter.From) {
			continue
		}
		if filter.To != nil && c.FirstSeen.After(*filter.To) {
			continue
		}
		if filter.MinPriority != nil && c.Priority < *filter.MinPriority {
			continue
		}
		if filter.SearchTemplate != "" && !strings.Contains(c.Template, filter.SearchTemplate) {
			continue
		}
		if len(filter.Services) > 0 && !overlapsServices(c.Services, filter.Services) {
			continue
		}
		if len(filter.Levels) > 0 && !levelsOverlap(c.Levels, filter.Levels) {
			continue
		}
		out = append(out, c)
	}

	switch filter.OrderBy {
	case "count_desc":
		sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	case "last_seen_desc":
		sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	default:
		sort.Slice(out, func(i, j int) bool {
			if out[i].Priority != out[j].Priority {
				return out[i].Priority > out[j].Priority
			}
			return out[i].LastSeen.After(out[j].LastSeen)
		})
	}

	total := int64(len(out))
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if filter.Offset >= len(out) {
		return nil, total, nil
	}
	out = out[filter.Offset:]
	if len(out) > limit {
		out = out[:limit]
	}
	return out, total, nil
}

func overlapsServices(have, want []string) bool {
	set := make(map[string]bool, len(want))
	for _, w := range want {
		set[w] = true
	}
	for _, h := range have {
		if set[h] {
			return true
		}
	}
	return false
}

func levelsOverlap(have map[model.Level]int64, want []model.Level) bool {
	for _, l := range want {
		if have[l] > 0 {
			return true
		}
	}
	return false
}

func (s *Store) PruneStaleClusters(_ context.Context, olderThan time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for id, c := range s.clusters {
		if c.LastSeen.Before(olderThan) {
			delete(s.clusters, id)
			delete(s.events, id)
			n++
		}
	}
	return n, nil
}

func (s *Store) SaveAnalysis(_ context.Context, a model.Analysis) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.SuggestedActions == nil {
		a.SuggestedActions = []string{}
	}
	if a.RelatedClusterIDs == nil {
		a.RelatedClusterIDs = []string{}
	}
	for i, existing := range s.analyses {
		if existing.ClusterID == a.ClusterID &&
			existing.WindowStart.Equal(a.WindowStart) &&
			existing.WindowEnd.Equal(a.WindowEnd) {
			s.analyses[i] = a
			return nil
		}
	}
	s.analyses = append(s.analyses, a)
	return nil
}

func (s *Store) LatestAnalysisForCluster(_ context.Context, clusterID string) (*model.Analysis, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest *model.Analysis
	for i := range s.analyses {
		a := s.analyses[i]
		if a.ClusterID != clusterID {
			continue
		}
		if latest == nil || a.WindowEnd.After(latest.WindowEnd) {
			cp := a
			latest = &cp
		}
	}
	return latest, nil
}

func (s *Store) ListRecentAnalyses(_ context.Context, limit int) ([]model.Analysis, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 20
	}
	out := make([]model.Analysis, len(s.analyses))
	copy(out, s.analyses)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
