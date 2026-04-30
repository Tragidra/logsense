// Package sqlite is the SQLite-backed implementation of storage.Repository
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/Tragidra/loglens/internal/storage"
	"github.com/Tragidra/loglens/internal/storage/migrator"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store is the SQLite-backed storage.Repository.
type Store struct {
	db     *sql.DB
	path   string
	logger *slog.Logger
}

// Config holds the connection settings
type Config struct {
	// Path is the SQLite file path, use ":memory:" for an in-memory database for testing.
	Path string
}

// New opens (or creates) the SQLite database, applies pending migrations, and returns a ready Store
func New(ctx context.Context, cfg Config, logger *slog.Logger) (*Store, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("sqlite: path is empty")
	}

	if cfg.Path != ":memory:" && !strings.HasPrefix(cfg.Path, "file:") {
		dir := filepath.Dir(cfg.Path)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("sqlite: mkdir %s: %w", dir, err)
			}
		}
	}

	dsn := buildDSN(cfg.Path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}

	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: ping: %w", err)
	}

	if err := migrator.Apply(ctx, db, migrationsFS, "migrations", migrator.DialectSQLite); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: migrations: %w", err)
	}

	logger.Info("sqlite: ready", "path", cfg.Path)

	return &Store{db: db, path: cfg.Path, logger: logger}, nil
}

// buildDSN appends our standard pragmas to the path
func buildDSN(path string) string {
	if path == ":memory:" {
		// In-memory needs a shared cache so the single-connection pool retains state across calls in test scenarios
		return "file::memory:?cache=shared&_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)"
	}

	pragmas := url.Values{}
	pragmas.Add("_pragma", "journal_mode(wal)")
	pragmas.Add("_pragma", "foreign_keys(on)")
	pragmas.Add("_pragma", "busy_timeout(5000)")
	pragmas.Add("_pragma", "synchronous(normal)")

	if strings.HasPrefix(path, "file:") {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		return path + sep + pragmas.Encode()
	}
	return "file:" + path + "?" + pragmas.Encode()
}

// Ping verifies the database is reachable
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close releases the database handle
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB { return s.db }

var _ storage.Repository = (*Store)(nil)
