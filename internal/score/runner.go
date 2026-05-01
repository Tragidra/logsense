package score

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Tragidra/logstruct/internal/config"
	"github.com/Tragidra/logstruct/internal/storage"
	"github.com/Tragidra/logstruct/model"
)

type activeCluster struct {
	cluster   *model.Cluster
	buf       *WindowBuffer
	firstSeen time.Time
}

// Runner scores clusters on a configurable tick using a sliding window buffer.
type Runner struct {
	scorer Scorer
	repo   storage.Repository
	logger *slog.Logger
	cfg    config.ScoreConfig
	mu     sync.Mutex
	active map[string]*activeCluster
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewRunner builds a Runner and starts the background scoring goroutine.
func NewRunner(cfg config.ScoreConfig, scorer Scorer, repo storage.Repository, logger *slog.Logger) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runner{
		scorer: scorer,
		repo:   repo,
		logger: logger,
		cfg:    cfg,
		active: make(map[string]*activeCluster),
		cancel: cancel,
	}
	r.wg.Add(1)
	go r.loop(ctx)
	return r
}

// Observe registers one event for cluster's ID so the runner can track its window stats.
func (r *Runner) Observe(c *model.Cluster, service string, level model.Level) {
	r.mu.Lock()
	ac, ok := r.active[c.ID]
	if !ok {
		ac = &activeCluster{
			cluster:   c,
			buf:       NewWindowBuffer(),
			firstSeen: c.FirstSeen,
		}
		r.active[c.ID] = ac
	}
	r.mu.Unlock()

	ac.buf.Record(service, level)
}

// Close stops the background goroutine and waits for it to exit.
func (r *Runner) Close() {
	r.cancel()
	r.wg.Wait()
}

func (r *Runner) loop(ctx context.Context) {
	defer r.wg.Done()
	window := r.cfg.Window.D()
	if window <= 0 {
		window = time.Minute
	}
	ticker := time.NewTicker(window)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.scoreAll(ctx)
		}
	}
}

func (r *Runner) scoreAll(ctx context.Context) {
	r.mu.Lock()
	snapshot := make(map[string]*activeCluster, len(r.active))
	for id, ac := range r.active {
		snapshot[id] = ac
	}
	r.mu.Unlock()

	for _, ac := range snapshot {
		countNow, avgRecent, services, dominant, total := ac.buf.Snapshot()

		w := WindowStats{
			CountNow:         countNow,
			CountAvgRecent:   avgRecent,
			CountTotal:       total,
			ServicesInWindow: services,
			Level:            dominant,
			FirstSeen:        ac.firstSeen,
		}

		priority, _ := r.scorer.Score(ctx, ac.cluster, w)

		ac.cluster.Priority = priority
		if err := r.repo.UpsertCluster(ctx, *ac.cluster); err != nil {
			r.logger.Error("score: upsert cluster failed", "cluster_id", ac.cluster.ID, "err", err)
		}
	}
}
