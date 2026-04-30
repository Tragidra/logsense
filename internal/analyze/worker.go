package analyze

import (
	"context"
	"log/slog"
	"sync"
)

// WorkerPool runs Analyze calls off the hot path. Submit returns immediately; workers process in the background.
// P.S. Drops requests when the queue is full.
type WorkerPool struct {
	analyzer Analyzer
	jobs     chan string
	logger   *slog.Logger
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewWorkerPool starts numWorkers goroutines processing up to queueSize pending jobs
func NewWorkerPool(a Analyzer, numWorkers, queueSize int, logger *slog.Logger) *WorkerPool {
	if numWorkers <= 0 {
		numWorkers = 2
	}
	if queueSize <= 0 {
		queueSize = 64
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &WorkerPool{
		analyzer: a,
		jobs:     make(chan string, queueSize),
		logger:   logger,
		cancel:   cancel,
	}
	for i := 0; i < numWorkers; i++ {
		p.wg.Add(1)
		go p.worker(ctx)
	}
	return p
}

// Submit enqueues a clusterID for analysis, returns false if the queue is full
func (p *WorkerPool) Submit(clusterID string) bool {
	select {
	case p.jobs <- clusterID:
		return true
	default:
		p.logger.Warn("analyze: queue full, dropping request", "cluster_id", clusterID)
		return false
	}
}

// Close stops accepting new jobs and waits for workers to drain
func (p *WorkerPool) Close() {
	close(p.jobs)
	p.cancel()
	p.wg.Wait()
}

func (p *WorkerPool) worker(ctx context.Context) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case clusterID, ok := <-p.jobs:
			if !ok {
				return
			}
			if _, err := p.analyzer.Analyze(ctx, clusterID); err != nil {
				p.logger.Error("analyze: worker call failed", "cluster_id", clusterID, "err", err)
			}
		}
	}
}
