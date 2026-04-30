package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tragidra/loglens/internal/config"
)

func TestMigrations(t *testing.T) {
	dsn := testDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Drop everything so we test migrations from scratch.
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS analyses CASCADE",
		"DROP TABLE IF EXISTS events CASCADE",
		"DROP TABLE IF EXISTS clusters CASCADE",
		"DROP TABLE IF EXISTS loglens_migrations CASCADE",
	} {
		bootstrapExec(t, ctx, dsn, stmt)
	}

	store, err := New(ctx, config.StorageConfig{DSN: dsn, MaxOpenConns: 4}, discardLogger())
	require.NoError(t, err)
	defer store.Close()

	require.NoError(t, store.Ping(ctx))

	for _, table := range []string{"clusters", "events", "analyses"} {
		t.Run("table_"+table, func(t *testing.T) {
			var exists bool
			err := store.Pool().QueryRow(ctx,
				`SELECT EXISTS (
					SELECT 1 FROM pg_tables
					WHERE schemaname = 'public' AND tablename = $1
				)`, table,
			).Scan(&exists)
			require.NoError(t, err)
			assert.True(t, exists, "table %s should exist", table)
		})
	}

	t.Run("idempotent", func(t *testing.T) {
		store2, err := New(ctx, config.StorageConfig{DSN: dsn}, discardLogger())
		require.NoError(t, err)
		require.NoError(t, store2.Close())
	})
}

func TestNew_BadDSN(t *testing.T) {
	_, err := New(context.Background(), config.StorageConfig{DSN: ""}, discardLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dsn is empty")
}

func TestEmbeddedMigrations(t *testing.T) {
	entries, err := migrationsFS.ReadDir("migrations")
	require.NoError(t, err)
	require.Len(t, entries, 4, "expected 4 migration files")

	wantNames := map[string]bool{
		"001_clusters.sql":                         true,
		"002_events.sql":                           true,
		"003_analyses.sql":                         true,
		"004_drop_clusters_fingerprint_unique.sql": true,
	}
	for _, e := range entries {
		assert.True(t, wantNames[e.Name()], "unexpected file: %s", e.Name())
		data, err := migrationsFS.ReadFile("migrations/" + e.Name())
		require.NoError(t, err)
		assert.NotContains(t, string(data), "-- +goose", "goose markers should be removed")
	}
}

// bootstrapExec runs a single SQL statement using a temporary store and sed to reset schema before migration tests.
func bootstrapExec(t *testing.T, ctx context.Context, dsn, stmt string) {
	t.Helper()
	store, err := New(ctx, config.StorageConfig{DSN: dsn}, discardLogger())
	require.NoError(t, err)
	defer store.Close()
	_, err = store.Pool().Exec(ctx, stmt)
	require.NoError(t, err, "bootstrap: %s", stmt)
}
