package postgres

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Tragidra/logstruct/internal/config"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping postgres integration test")
	}
	return dsn
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := testDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	store, err := New(ctx, config.StorageConfig{DSN: dsn, MaxOpenConns: 4}, discardLogger())
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	for _, stmt := range []string{
		`TRUNCATE TABLE clusters CASCADE`,
		`TRUNCATE TABLE events`,
	} {
		_, err := store.Pool().Exec(ctx, stmt)
		require.NoError(t, err, "truncate: %s", stmt)
	}

	return store
}
