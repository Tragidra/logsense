// Package postgres is the PostgreSQL-backed implementation of storage.Repository
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Tragidra/logstruct/internal/config"
	"github.com/Tragidra/logstruct/internal/storage"
	"github.com/Tragidra/logstruct/internal/storage/migrator"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store is the Postgres-backed Repository implementation.
type Store struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func New(ctx context.Context, cfg config.StorageConfig, logger *slog.Logger) (*Store, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("postgres: storage.dsn is empty")
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		poolCfg.MaxConns = int32(cfg.MaxOpenConns)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	if err := runMigrations(ctx, sqlDB, logger); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: migrations: %w", err)
	}

	return &Store{pool: pool, logger: logger}, nil
}

func runMigrations(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	if err := migrator.Apply(ctx, db, migrationsFS, "migrations", migrator.DialectPostgres); err != nil {
		return err
	}
	logger.Info("postgres: migrations applied")
	return nil
}

// Ping verifies the connection to Postgres
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Close releases all pool connections
func (s *Store) Close() error {
	s.pool.Close()
	return nil
}

// DB returns a *sql.DB backed by the pool for use in tests and utilities only
func (s *Store) DB() *sql.DB {
	return stdlib.OpenDBFromPool(s.pool)
}

// Pool exposes the underlying pgxpool.Pool for cross-package test helpers
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// Ensure Store satisfies storage.Repository at compile time.
var _ storage.Repository = (*Store)(nil)
