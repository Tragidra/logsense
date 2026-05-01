package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tragidra/logstruct/internal/config"
	"github.com/Tragidra/logstruct/internal/storage"
	"github.com/Tragidra/logstruct/internal/storage/postgres"
	"github.com/Tragidra/logstruct/internal/storage/sqlite"
)

// openStore opens the storage backend selected by the flags, one of dbPath or postgresDSN must be non-empty. Migrations are applied automatically by the underlying constructors.
func openStore(ctx context.Context, dbPath, postgresDSN string, logger *slog.Logger) (storage.Repository, error) {
	if dbPath != "" && postgresDSN != "" {
		return nil, errors.New("--db and --postgres are mutually exclusive")
	}
	if dbPath == "" && postgresDSN == "" {
		return nil, errors.New("one of --db or --postgres is required")
	}

	if postgresDSN != "" {
		return postgres.New(ctx, config.StorageConfig{Kind: "postgres", DSN: postgresDSN}, logger)
	}
	return sqlite.New(ctx, sqlite.Config{Path: dbPath}, logger)
}

func describeStore(dbPath, postgresDSN string) string {
	if postgresDSN != "" {
		return fmt.Sprintf("postgres (%s)", postgresDSN)
	}
	return fmt.Sprintf("sqlite (%s)", dbPath)
}
