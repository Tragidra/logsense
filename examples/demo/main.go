// Demo:
//  1. Appends 10 random log lines to ./loglens-demo.log (file in repo root) every 30 seconds.
//  2. Tails that file with LogLens (file source (.log) and SQLite storage and logsense-ai (local openai/gpt-oss-20b).
//  3. Serves the embedded Vue dashboard at http://localhost:8765.
//
// Requires a local OpenAI-compatible LLM server on http://localhost:7090/v1 (or http://localhost:1234/v1)
// (tested with openai/gpt-oss-20b). Set LOGSENSE_BASE_URL to override.
//
// This example lives inside the loglens module, the import
// "github.com/Tragidra/loglens" resolves to the local source — no separate
// download. In production code you would run
// `loglens ui --db ./loglens-demo.db` as a separate binary instead.
// Run from the repo root:
//
//	go run ./examples/demo
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Tragidra/loglens"
	"github.com/Tragidra/loglens/internal/analyze"
	"github.com/Tragidra/loglens/internal/api"
	"github.com/Tragidra/loglens/internal/config"
	"github.com/Tragidra/loglens/internal/llm/logsenseai"
	"github.com/Tragidra/loglens/internal/storage/sqlite"
	"github.com/Tragidra/loglens/web"
)

const (
	logPath  = "./loglens-demo.log"
	dbPath   = "./loglens-demo.db"
	httpAddr = ":8765"
	tickRate = 30 * time.Second
	burstQty = 10
)

// templates are intentionally varied so Drain forms several distinct clusters.
var templates = []string{
	`{"level":"info","msg":"request handled","route":"/api/users/%d","ms":%d}`,
	`{"level":"info","msg":"request handled","route":"/api/orders/%d","ms":%d}`,
	`{"level":"warn","msg":"slow query","table":"orders","ms":%d,"id":%d}`,
	`{"level":"error","msg":"payment gateway timeout","order":%d,"attempt":%d}`,
	`{"level":"error","msg":"redis dial failed","host":"cache-%d","retry":%d}`,
	`{"level":"fatal","msg":"out of memory","worker":%d,"alloc_mb":%d}`,
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	baseURL := "http://localhost:7090/v1"
	if v := os.Getenv("LOGSENSE_BASE_URL"); v != "" {
		baseURL = v
	}

	// Fresh state on every run
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + "-shm")
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(logPath)
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		logger.Error("create log file", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Logsense-ai forces temperature=0 for stable structured JSON output from local models
	ll, err := loglens.New(loglens.Config{
		Sources: []loglens.SourceConfig{{
			Kind:      "file",
			Path:      logPath,
			Service:   "demo",
			Format:    "json",
			StartFrom: "beginning",
		}},
		Storage: loglens.StorageConfig{Kind: "sqlite", SQLitePath: dbPath},
		AI: loglens.AIConfig{
			Provider: "logsense-ai",
			BaseURL:  baseURL,
			Timeout:  60 * time.Second,
		},
		Logger: logger,
	})
	if err != nil {
		logger.Error("loglens.New", "err", err)
		os.Exit(1)
	}
	defer ll.Close()
	if err := ll.Start(ctx); err != nil {
		logger.Error("loglens.Start", "err", err)
		os.Exit(1)
	}

	// Open a SECOND SQLite handle against the same file for the read-only API.
	apiStore, err := sqlite.New(ctx, sqlite.Config{Path: dbPath}, logger)
	if err != nil {
		logger.Error("api store", "err", err)
		os.Exit(1)
	}
	defer apiStore.Close()

	llmCfg := &config.LLMConfig{
		Provider:    "logsense-ai",
		BaseURL:     baseURL,
		Timeout:     config.Duration(60 * time.Second),
		MaxRetries:  2,
		MaxTokens:   2000,
		Temperature: 0, // forced for local models
	}
	provider, err := logsenseai.New(llmCfg, logger)
	if err != nil {
		logger.Error("logsenseai provider", "err", err)
		os.Exit(1)
	}

	analyzer := analyze.New(config.AnalyzeConfig{
		Enabled:       true,
		MaxConcurrent: 1,
		CacheTTL:      config.Duration(15 * time.Minute),
		Window:        config.Duration(10 * time.Minute),
	}, provider, apiStore, logger)

	apiSrv := api.New(config.APIConfig{Addr: httpAddr}, apiStore, analyzer, logger)

	mux := http.NewServeMux()
	mux.Handle("/api/", apiSrv.Handler())
	mux.Handle("/", web.Handler())

	httpSrv := &http.Server{
		Addr:         httpAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 90 * time.Second, // longer for LLM calls in analyze
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		writeBurst(logger) // initial burst so the dashboard isn't empty on load
		t := time.NewTicker(tickRate)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				writeBurst(logger)
			}
		}
	}()

	go func() {
		defer wg.Done()
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	logger.Info("demo: ready",
		"dashboard", "http://localhost"+httpAddr,
		"llm", baseURL,
		"log_file", logPath,
		"db", dbPath,
		"tick", tickRate.String(),
	)

	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("http", "err", err)
		stop()
	}
	wg.Wait()
	logger.Info("demo: stopped", "dropped", ll.Stats().Dropped)
}

// writeBurst appends burstQty templated lines to logPath.
func writeBurst(logger *slog.Logger) {
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		logger.Warn("write burst: open", "err", err)
		return
	}
	defer f.Close()

	for i := 0; i < burstQty; i++ {
		tmpl := templates[rand.Intn(len(templates))]
		line := fmt.Sprintf(tmpl, rand.Intn(10000), rand.Intn(2000))
		if _, err := fmt.Fprintln(f, line); err != nil {
			logger.Warn("write burst: append", "err", err)
			return
		}
	}
	logger.Info("demo: appended burst", "lines", burstQty)
}
