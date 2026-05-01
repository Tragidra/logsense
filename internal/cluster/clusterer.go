package cluster

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Tragidra/logstruct/internal/config"
	"github.com/Tragidra/logstruct/internal/storage"
	"github.com/Tragidra/logstruct/model"
)

// Clusterer groups log events into clusters and persists results.
type Clusterer interface {
	Ingest(ctx context.Context, e model.LogEvent) (*model.Cluster, error)
	Close() error
}

type defaultClusterer struct {
	tree   *Tree
	repo   storage.Repository
	logger *slog.Logger
	cfg    config.ClusterConfig
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New builds a Clusterer from config.
func New(cfg config.ClusterConfig, repo storage.Repository, logger *slog.Logger) (Clusterer, error) {
	maxDepth := cfg.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 3
	}
	maxChildren := cfg.MaxChildrenPerNode
	if maxChildren <= 0 {
		maxChildren = 100
	}
	maxTemplates := cfg.MaxTemplatesPerLeaf
	if maxTemplates <= 0 {
		maxTemplates = 10
	}
	sim := cfg.SimilarityThreshold
	if sim <= 0 {
		sim = 0.4
	}

	tree := NewTree(maxDepth, maxChildren, maxTemplates, sim)

	bgCtx, cancel := context.WithCancel(context.Background())
	c := &defaultClusterer{
		tree: tree, repo: repo, logger: logger, cfg: cfg, cancel: cancel,
	}

	if cfg.PruneAfter.D() > 0 {
		c.wg.Add(1)
		go c.pruneLoop(bgCtx)
	}

	return c, nil
}

// Ingest upserts its cluster to the repository and saves the event.
func (c *defaultClusterer) Ingest(ctx context.Context, e model.LogEvent) (*model.Cluster, error) {
	g := c.tree.Insert(e)
	cl := logGroupToCluster(g)

	if err := c.repo.UpsertCluster(ctx, cl); err != nil {
		return nil, fmt.Errorf("cluster ingest: upsert cluster: %w", err)
	}
	if err := c.repo.SaveEvent(ctx, e, g.ID); err != nil {
		return nil, fmt.Errorf("cluster ingest: save event: %w", err)
	}
	return &cl, nil
}

// Close stops background goroutines and waits for them to finish.
func (c *defaultClusterer) Close() error {
	c.cancel()
	c.wg.Wait()
	return nil
}

func (c *defaultClusterer) pruneLoop(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.tree.Prune(c.cfg.PruneAfter.D())
			c.logger.Info("cluster: pruned stale groups")
		}
	}
}

func logGroupToCluster(g *LogGroup) model.Cluster {
	services := make([]string, 0, len(g.Services))
	for s := range g.Services {
		services = append(services, s)
	}
	sort.Strings(services)

	tmpl := strings.Join(g.TokensTmpl, " ")
	h := fnv.New64a()
	h.Write([]byte(tmpl))
	fp := fmt.Sprintf("%016x", h.Sum64())

	examples := g.Examples.Items()
	if examples == nil {
		examples = []string{}
	}
	if services == nil {
		services = []string{}
	}
	// So the returned cluster is independent of the tree's internal LogGroup
	//which may be mutated concurrently on subsequent ingests.
	levels := make(map[model.Level]int64, len(g.Levels))
	for k, v := range g.Levels {
		levels[k] = v
	}

	return model.Cluster{
		ID:             g.ID,
		Fingerprint:    fp,
		Template:       tmpl,
		TemplateTokens: g.TokensTmpl,
		AnomalyFlags:   []string{},
		Services:       services,
		Levels:         levels,
		Count:          g.Count,
		FirstSeen:      g.FirstSeen,
		LastSeen:       g.LastSeen,
		ExamplesSample: examples,
	}
}
