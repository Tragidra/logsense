package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/Tragidra/logsense/internal/analyze"
	"github.com/Tragidra/logsense/internal/api"
	"github.com/Tragidra/logsense/internal/config"
	"github.com/Tragidra/logsense/internal/llm"
	"github.com/Tragidra/logsense/internal/llm/fake"
	"github.com/Tragidra/logsense/internal/llm/logsenseai"
	"github.com/Tragidra/logsense/internal/llm/openrouter"
	"github.com/Tragidra/logsense/web"
)

var (
	uiConfig    string
	uiDB        string
	uiPostgres  string
	uiAddr      string
	uiNoBrowser bool
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Serve the read-only Dashboard",
	Long: `Opens an existing logsense store and serves the Dashboard UI.

Storage is selected via --db (SQLite, default) or --postgres (DSN).

Pass --config to point at a YAML file containing the
llm: section; without it, a fake provider is used and analyze actions return
canned responses.`,
	Args: cobra.NoArgs,
	RunE: runUI,
}

func init() {
	uiCmd.Flags().StringVar(&uiConfig, "config", "", "optional YAML config (for AI provider settings)")
	uiCmd.Flags().StringVar(&uiDB, "db", "./logsense.db", "SQLite database path")
	uiCmd.Flags().StringVar(&uiPostgres, "postgres", "", "Postgres DSN (alternative to --db)")
	uiCmd.Flags().StringVar(&uiAddr, "addr", ":8765", "HTTP listen address")
	uiCmd.Flags().BoolVar(&uiNoBrowser, "no-browser", false, "do not auto-open the browser")
	rootCmd.AddCommand(uiCmd)
}

func runUI(_ *cobra.Command, _ []string) error {
	logger := newLogger()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := loadUIConfig(uiConfig)
	if err != nil {
		return err
	}

	store, err := openStore(ctx, uiDB, uiPostgres, logger)
	if err != nil {
		return err
	}
	defer store.Close()

	provider, err := buildUIProvider(cfg.LLM, logger)
	if err != nil {
		return fmt.Errorf("llm: %w", err)
	}

	analyzer := analyze.New(cfg.Analyze, provider, store, logger)

	apiSrv := api.New(cfg.API, store, analyzer, logger)

	mux := http.NewServeMux()
	mux.Handle("/api/", apiSrv.Handler())
	mux.Handle("/", web.Handler())

	server := &http.Server{
		Addr:         uiAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	logger.Info("logsense ui: ready",
		"addr", uiAddr,
		"store", describeStore(uiDB, uiPostgres),
		"ai_provider", providerLabel(cfg.LLM.Provider))

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("ui server: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		<-gctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("ui server: shutdown", "err", err)
		}
		return nil
	})

	if !uiNoBrowser {
		go func() {
			time.Sleep(300 * time.Millisecond)
			url := "http://localhost" + normalizeAddr(uiAddr)
			if err := openBrowser(url); err != nil {
				logger.Warn("logsense ui: could not open browser", "url", url, "err", err)
			}
		}()
	}

	if err := g.Wait(); err != nil && ctx.Err() == nil {
		return err
	}
	logger.Info("logsense ui: shutdown complete")
	return nil
}

// loadUIConfig returns a Config populated either from the file at path or from defaults if path is empty.
func loadUIConfig(path string) (*config.Config, error) {
	if path == "" {
		cfg := &config.Config{}
		applyUIDefaults(cfg)
		return cfg, nil
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	applyUIDefaults(cfg)
	return cfg, nil
}

func applyUIDefaults(cfg *config.Config) {
	if cfg.LLM.Provider == "" {
		cfg.LLM.Provider = "fake"
	}
}

func buildUIProvider(cfg config.LLMConfig, logger *slog.Logger) (llm.Provider, error) {
	switch cfg.Provider {
	case "openrouter":
		return openrouter.New(&cfg, logger)
	case "logsense-ai":
		return logsenseai.New(&cfg, logger)
	case "fake", "":
		logger.Warn("this is a fake ai provider")
		return fake.New(), nil
	default:
		return nil, fmt.Errorf("unknown llm provider %q", cfg.Provider)
	}
}

func providerLabel(p string) string {
	if p == "" {
		return "fake"
	}
	return p
}

func normalizeAddr(addr string) string {
	if addr == "" {
		return ":8765"
	}
	if addr[0] == ':' {
		return addr
	}
	for i := 0; i < len(addr); i++ {
		if addr[i] == ':' {
			return addr[i:]
		}
	}
	return ":" + addr
}
