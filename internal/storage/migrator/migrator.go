// Package migrator runs ordered SQL migrations from an embedded filesystem.
//
// The migrator is intentionally tiny: list *.sql files in lexical order, apply each in a single transaction,
// and record the applied version in a tracking table.
//
// Both SQLite and Postgres are supported via a Dialect parameter that selects
// the placeholder style for the tracking-table inserts.
//
// Migration files are named "XXX_description.sql" where XXX is a zero-padded numeric prefix used for ordering.
package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// Dialect selects the SQL placeholder style for parameterised statements.
type Dialect string

const (
	// DialectSQLite uses "?" placeholders.
	DialectSQLite Dialect = "sqlite"
	// DialectPostgres uses "$1, $2, ..." placeholders.
	DialectPostgres Dialect = "postgres"
)

// Migration is a single SQL script with its ordering version.
type Migration struct {
	Version string // e.g. "001"
	Name    string // file name without the .sql suffix
	SQL     string
}

// Apply runs every migration in fsys (recursively under dir) that has not yet been applied to db.
// SQL statements are executed inside a transaction per migration so a failure rolls back atomically.
func Apply(ctx context.Context, db *sql.DB, fsys fs.FS, dir string, dialect Dialect) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS logstruct_migrations (
			version    TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)`); err != nil {
		return fmt.Errorf("migrator: create tracking table: %w", err)
	}

	applied, err := loadApplied(ctx, db)
	if err != nil {
		return err
	}

	migrations, err := discover(fsys, dir)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}
		if err := applyOne(ctx, db, m, dialect); err != nil {
			return fmt.Errorf("migrator: apply %s: %w", m.Version, err)
		}
	}
	return nil
}

func loadApplied(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM logstruct_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrator: list applied: %w", err)
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("migrator: scan applied: %w", err)
		}
		out[v] = true
	}
	return out, rows.Err()
}

func discover(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("migrator: read dir %s: %w", dir, err)
	}

	var out []Migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".sql")
		idx := strings.IndexByte(base, '_')
		if idx <= 0 {
			return nil, fmt.Errorf("migrator: %s: expected NNN_description.sql", e.Name())
		}
		version := base[:idx]

		raw, err := fs.ReadFile(fsys, dir+"/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("migrator: read %s: %w", e.Name(), err)
		}
		out = append(out, Migration{
			Version: version,
			Name:    base,
			SQL:     string(raw),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

func applyOne(ctx context.Context, db *sql.DB, m Migration, dialect Dialect) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	insertSQL := fmt.Sprintf(
		`INSERT INTO logstruct_migrations (version, name, applied_at) VALUES (%s, %s, %s)`,
		placeholder(dialect, 1), placeholder(dialect, 2), placeholder(dialect, 3),
	)
	if _, err := tx.ExecContext(ctx, insertSQL, m.Version, m.Name, nowISO()); err != nil {
		return fmt.Errorf("record version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func placeholder(dialect Dialect, n int) string {
	if dialect == DialectPostgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

func nowISO() string {
	return nowISOImpl()
}

var ErrNoMigrations = errors.New("migrator: no migrations found")
