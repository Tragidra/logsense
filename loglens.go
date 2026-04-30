// Package logsense is a Go library for AI-powered log analysis.
//
// logsense embeds into a Go service to ingest logs from files or via inline reporting, group them into templates
// using the Drain algorithm, score each cluster by anomaly signals, and use a configured LLM provider to produce
// summaries, root-cause hypotheses, and suggested actions.
//
// Quickstart:
//
//	ll, err := logsense.New(logsense.Config{
//	    Sources: []logsense.SourceConfig{
//	        {Kind: "file", Path: "/var/log/myapp.log"},
//	    },
//	    AI: logsense.AIConfig{Provider: "logsense-ai"},
//	})
//	if err != nil { return err }
//	defer ll.Close()
//	if err := ll.Start(ctx); err != nil { return err }
//
// Storage defaults to a local SQLite file (./logsense.db). For Postgres, set:
// Storage.Kind = "postgres" and provide a DSN.
//
// View clusters and analyses with the logsense binary:
//
//	go install github.com/Tragidra/logsense/cmd/logsense@latest
//	logsense ui --db ./logsense.db
//
// See https://github.com/Tragidra/logsense for full documentation
package logsense

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/Tragidra/logsense/internal/analyze"
	"github.com/Tragidra/logsense/internal/cluster"
	internalconfig "github.com/Tragidra/logsense/internal/config"
	"github.com/Tragidra/logsense/internal/ingest"
	"github.com/Tragidra/logsense/internal/ingest/file"
	"github.com/Tragidra/logsense/internal/llm"
	"github.com/Tragidra/logsense/internal/llm/fake"
	"github.com/Tragidra/logsense/internal/llm/logsenseai"
	"github.com/Tragidra/logsense/internal/llm/openrouter"
	"github.com/Tragidra/logsense/internal/normalize"
	"github.com/Tragidra/logsense/internal/score"
	"github.com/Tragidra/logsense/internal/storage"
	"github.com/Tragidra/logsense/internal/storage/memory"
	"github.com/Tragidra/logsense/internal/storage/postgres"
	"github.com/Tragidra/logsense/internal/storage/sqlite"
	"github.com/Tragidra/logsense/model"
)

// rawChannelSize is the buffer size of the source -> pipeline channel. When full, Report() drops events rather
// than blocking the caller.
const rawChannelSize = 512

// logsense is the running library instance. Construct with New() or NewFromYAML(); start with Start();
// release with Close().
type logsense struct {
	cfg    Config
	logger *slog.Logger

	repo        storage.Repository
	provider    llm.Provider
	clusterer   cluster.Clusterer
	normalizer  normalize.Normalizer
	scoreRunner *score.Runner
	analyzer    analyze.Analyzer
	pool        *analyze.WorkerPool
	sources     []ingest.Source

	rawCh   chan model.RawLog
	dropped atomic.Int64

	startOnce sync.Once
	closeOnce sync.Once
	startErr  error

	cancel context.CancelFunc
	g      *errgroup.Group
	gctx   context.Context

	// state flags. running is set true between Start() and Close().
	running atomic.Bool
}

// Stats reports cumulative counters useful for monitoring the library from the
// host service.
type Stats struct {
	// Dropped counts events that Report() could not enqueue because the pipeline channel was full
	Dropped int64
}

// New constructs a logsense instance from a Config. It does not start any goroutines - call Start() for that.
func New(cfg Config) (*logsense, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	logger := cfg.Logger

	repo, err := buildRepository(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("logsense: storage: %w", err)
	}

	provider, err := buildProvider(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("logsense: ai provider: %w", err)
	}

	internal := cfg.toInternal()

	clusterer, err := cluster.New(internal.Cluster, repo, logger)
	if err != nil {
		_ = repo.Close()
		return nil, fmt.Errorf("logsense: cluster: %w", err)
	}

	normalizer := normalize.New()

	scorer := score.New(internal.Score)
	scoreRunner := score.NewRunner(internal.Score, scorer, repo, logger)

	analyzer := analyze.New(internal.Analyze, provider, repo, logger)
	pool := analyze.NewWorkerPool(analyzer, internal.Analyze.MaxConcurrent, 64, logger)

	sources, err := buildSources(internal.Sources, logger)
	if err != nil {
		scoreRunner.Close()
		pool.Close()
		_ = clusterer.Close()
		_ = repo.Close()
		return nil, fmt.Errorf("logsense: sources: %w", err)
	}

	return &logsense{
		cfg:         cfg,
		logger:      logger,
		repo:        repo,
		provider:    provider,
		clusterer:   clusterer,
		normalizer:  normalizer,
		scoreRunner: scoreRunner,
		analyzer:    analyzer,
		pool:        pool,
		sources:     sources,
		rawCh:       make(chan model.RawLog, rawChannelSize),
	}, nil
}

// NewFromYAML loads configuration from a YAML file (with ${VAR} env expansion) and constructs a logsense instance
func NewFromYAML(path string) (*logsense, error) {
	cfg, err := loadYAMLConfig(path)
	if err != nil {
		return nil, err
	}
	return New(cfg)
}

// Start spins up the source readers, the pipeline goroutine, and (if enabled) the inline AI worker pool.
// It is non-blocking: returns once goroutines are launched.
//
// Calling Start more than once is a no-op (the first call's error is returned).
func (ll *logsense) Start(ctx context.Context) error {
	ll.startOnce.Do(func() {
		gctx, cancel := context.WithCancel(ctx)
		g, gctx := errgroup.WithContext(gctx)
		ll.gctx = gctx
		ll.g = g
		ll.cancel = cancel
		ll.running.Store(true)

		for _, s := range ll.sources {
			src := s
			g.Go(func() error {
				ll.logger.Info("logsense: source starting", "name", src.Name())
				if err := src.Stream(gctx, ll.rawCh); err != nil && gctx.Err() == nil {
					return fmt.Errorf("source %s: %w", src.Name(), err)
				}
				return nil
			})
		}

		g.Go(func() error {
			return ll.pipelineLoop(gctx)
		})
	})
	return ll.startErr
}

// Close stops all goroutines and releases storage. Idempotent. Safe to call without a prior Start().
//
// Close blocks for up to 5 seconds waiting for in-flight pipeline work to drain,
// pending Report() calls beyond the buffer are dropped.
func (ll *logsense) Close() error {
	var err error
	ll.closeOnce.Do(func() {
		ll.running.Store(false)
		if ll.cancel != nil {
			ll.cancel()
		}

		if ll.g != nil {
			done := make(chan error, 1)
			go func() { done <- ll.g.Wait() }()
			select {
			case e := <-done:
				if e != nil && !errors.Is(e, context.Canceled) {
					err = e
				}
			case <-time.After(5 * time.Second):
				ll.logger.Warn("logsense: pipeline drain timed out after 5s")
			}
		}

		if ll.pool != nil {
			ll.pool.Close()
		}
		if ll.scoreRunner != nil {
			ll.scoreRunner.Close()
		}
		if ll.clusterer != nil {
			if e := ll.clusterer.Close(); e != nil && err == nil {
				err = e
			}
		}
		if ll.repo != nil {
			if e := ll.repo.Close(); e != nil && err == nil {
				err = e
			}
		}
	})
	return err
}

// Stats returns a snapshot of internal counters
func (ll *logsense) Stats() Stats {
	return Stats{Dropped: ll.dropped.Load()}
}

// pipelineLoop consumes raw events, normalizes, clusters, and observes for scoring.
// Runs until ctx cancels or the channel is closed.
func (ll *logsense) pipelineLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case raw, ok := <-ll.rawCh:
			if !ok {
				return nil
			}
			event := ll.normalizer.Normalize(raw)
			c, err := ll.clusterer.Ingest(ctx, event)
			if err != nil {
				ll.logger.Error("logsense: cluster ingest", "err", err)
				continue
			}
			ll.scoreRunner.Observe(c, event.Service, event.Level)

			if ll.cfg.Inline.Enabled && c != nil && c.Priority >= ll.cfg.Inline.MinPriority {
				if !ll.pool.Submit(c.ID) {
					ll.logger.Warn("logsense: analyze queue full, dropping cluster",
						"cluster_id", c.ID, "priority", c.Priority)
				}
			}
		}
	}
}

// buildRepository selects a storage backend.
func buildRepository(cfg Config, logger *slog.Logger) (storage.Repository, error) {
	switch cfg.Storage.Kind {
	case "memory":
		logger.Info("logsense: storage = memory (data is not persisted)")
		return memory.New(), nil
	case "postgres":
		return postgres.New(context.Background(), cfg.toInternal().Storage, logger)
	case "sqlite":
		return sqlite.New(context.Background(), sqlite.Config{Path: cfg.Storage.SQLitePath}, logger)
	default:
		return nil, fmt.Errorf("unknown storage kind %q", cfg.Storage.Kind)
	}
}

// buildProvider selects an LLM provider.
func buildProvider(cfg Config, logger *slog.Logger) (llm.Provider, error) {
	internal := cfg.toInternal().LLM
	switch cfg.AI.Provider {
	case "logsense-ai":
		return logsenseai.New(&internal, logger)
	case "openrouter":
		return openrouter.New(&internal, logger)
	case "fake":
		return fake.New(), nil
	default:
		return nil, fmt.Errorf("unknown ai provider %q", cfg.AI.Provider)
	}
}

// buildSources translates internal source configs into source readers.
func buildSources(specs []internalconfig.SourceConfig, logger *slog.Logger) ([]ingest.Source, error) {
	out := make([]ingest.Source, 0, len(specs))
	for i := range specs {
		spec := &specs[i]
		if spec.Kind != "file" {
			return nil, fmt.Errorf("source %s: unsupported kind %q (only \"file\" is supported)", spec.Name, spec.Kind)
		}
		s, err := file.New(spec, logger)
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", spec.Name, err)
		}
		out = append(out, s)
	}
	return out, nil
}
