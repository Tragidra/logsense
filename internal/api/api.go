package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Tragidra/logstruct/internal/analyze"
	"github.com/Tragidra/logstruct/internal/config"
	"github.com/Tragidra/logstruct/internal/storage"
)

// Server is the HTTP API server
type Server struct {
	cfg     config.APIConfig
	httpSrv *http.Server
	logger  *slog.Logger
}

// New builds a Server with all routes mounted.
func New(cfg config.APIConfig, repo storage.Repository, analyzer analyze.Analyzer, logger *slog.Logger) *Server {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(slogMiddleware(logger))

	if len(cfg.AllowedOrigins) > 0 {
		r.Use(corsMiddleware(cfg.AllowedOrigins))
	}

	r.Get("/api/healthz", healthz)
	r.Get("/api/readyz", newReadyHandler(repo).readyz)

	// Clusters
	ch := newClustersHandler(repo, analyzer, logger)
	r.Get("/api/clusters", ch.list)
	r.Get("/api/clusters/{id}", ch.get)
	r.Get("/api/clusters/{id}/events", ch.events)
	r.Post("/api/clusters/{id}/analyze", ch.triggerAnalyze)

	addr := cfg.Addr
	if addr == "" {
		addr = ":8080"
	}

	return &Server{
		cfg:    cfg,
		logger: logger,
		httpSrv: &http.Server{
			Addr:    addr,
			Handler: r,
		},
	}
}

// Handler returns the underlying http.Handler (useful for testing)
func (s *Server) Handler() http.Handler { return s.httpSrv.Handler }

// Start begins listening, but it blocks until the server stops.
func (s *Server) Start() error {
	s.logger.Info("api: listening", "addr", s.httpSrv.Addr)
	if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("api: listen: %w", err)
	}
	return nil
}

// Shutdown gracefully drains connections
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code string, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": msg},
	})
}
