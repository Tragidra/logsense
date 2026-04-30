package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Tragidra/loglens/internal/storage"
	"github.com/Tragidra/loglens/model"
)

const clusterColumns = `
	id, fingerprint, template,
	first_seen, last_seen, count, priority,
	anomaly_flags, services, levels_json, examples_sample`

// UpsertCluster inserts a new cluster or updates all mutable fields when a row with the same id already exists.
// Cluster ID (assigned once by the in-memory Drain tree) is the stable key; fingerprint is mutable because
// Drain generalizes templates as new events match a group.
func (s *Store) UpsertCluster(ctx context.Context, c model.Cluster) error {
	flags := c.AnomalyFlags
	if flags == nil {
		flags = []string{}
	}
	services := c.Services
	if services == nil {
		services = []string{}
	}
	examples := c.ExamplesSample
	if examples == nil {
		examples = []string{}
	}

	flagsJSON, err := json.Marshal(flags)
	if err != nil {
		return fmt.Errorf("upsert cluster: marshal flags: %w", err)
	}
	servicesJSON, err := json.Marshal(services)
	if err != nil {
		return fmt.Errorf("upsert cluster: marshal services: %w", err)
	}
	examplesJSON, err := json.Marshal(examples)
	if err != nil {
		return fmt.Errorf("upsert cluster: marshal examples: %w", err)
	}
	levelsJSON, err := json.Marshal(c.Levels)
	if err != nil {
		return fmt.Errorf("upsert cluster: marshal levels: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO clusters (
			id, fingerprint, template,
			first_seen, last_seen, count, priority,
			anomaly_flags, services, levels_json, examples_sample,
			updated_at
		) VALUES (
			$1, $2, $3,
			$4, $5, $6, $7,
			$8, $9, $10, $11,
			now()
		)
		ON CONFLICT (id) DO UPDATE SET
			fingerprint      = EXCLUDED.fingerprint,
			template         = EXCLUDED.template,
			last_seen        = EXCLUDED.last_seen,
			count            = EXCLUDED.count,
			priority         = EXCLUDED.priority,
			anomaly_flags    = EXCLUDED.anomaly_flags,
			services         = EXCLUDED.services,
			levels_json      = EXCLUDED.levels_json,
			examples_sample  = EXCLUDED.examples_sample,
			updated_at       = now()`,
		c.ID, c.Fingerprint, c.Template,
		c.FirstSeen, c.LastSeen, c.Count, c.Priority,
		string(flagsJSON), string(servicesJSON), string(levelsJSON), string(examplesJSON),
	)
	if err != nil {
		return fmt.Errorf("upsert cluster: %w", err)
	}
	return nil
}

// GetCluster returns a cluster by its ID.
func (s *Store) GetCluster(ctx context.Context, id string) (model.Cluster, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+clusterColumns+` FROM clusters WHERE id = $1`, id)
	c, err := scanCluster(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Cluster{}, storage.ErrNotFound
	}
	if err != nil {
		return model.Cluster{}, fmt.Errorf("get cluster %s: %w", id, err)
	}
	return c, nil
}

// ListClusters returns clusters matching the filter, plus the total row count.
// Services and Levels filters are applied in Go after scanning because they would require a JSON parser in SQL;
// both columns are TEXT JSON to keep the schema identical between SQLite and Postgres, the reported total is
// the pre-filter count for that reason.
func (s *Store) ListClusters(ctx context.Context, filter storage.ClusterFilter) ([]model.Cluster, int64, error) {
	conds, args := buildClusterWhere(filter)
	whereSQL := "WHERE " + strings.Join(conds, " AND ")

	var total int64
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM clusters `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("list clusters count: %w", err)
	}

	orderSQL := clusterOrderBy(filter.OrderBy)

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+clusterColumns+` FROM clusters `+whereSQL+
			` ORDER BY `+orderSQL+
			fmt.Sprintf(` LIMIT %d OFFSET %d`, limit, filter.Offset),
		args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list clusters query: %w", err)
	}
	defer rows.Close()

	var out []model.Cluster
	for rows.Next() {
		c, err := scanCluster(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("list clusters scan: %w", err)
		}
		if !servicesMatch(c.Services, filter.Services) {
			continue
		}
		if !levelsMatch(c.Levels, filter.Levels) {
			continue
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list clusters rows: %w", err)
	}
	return out, total, nil
}

// PruneStaleClusters deletes clusters whose last_seen is before olderThan, returns the number of rows deleted.
func (s *Store) PruneStaleClusters(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM clusters WHERE last_seen < $1`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("prune clusters: %w", err)
	}
	return tag.RowsAffected(), nil
}

// buildClusterWhere builds the SQL-side conditions. Services and Levels are not handled here; see ListClusters for why.
func buildClusterWhere(f storage.ClusterFilter) ([]string, []any) {
	conds := []string{"1=1"}
	args := []any{}
	n := 1

	if f.From != nil {
		conds = append(conds, fmt.Sprintf("last_seen >= $%d", n))
		args = append(args, *f.From)
		n++
	}
	if f.To != nil {
		conds = append(conds, fmt.Sprintf("last_seen <= $%d", n))
		args = append(args, *f.To)
		n++
	}
	if f.MinPriority != nil {
		conds = append(conds, fmt.Sprintf("priority >= $%d", n))
		args = append(args, *f.MinPriority)
		n++
	}
	if f.SearchTemplate != "" {
		conds = append(conds, fmt.Sprintf("template ILIKE $%d", n))
		args = append(args, "%"+f.SearchTemplate+"%")
	}

	return conds, args
}

func clusterOrderBy(orderBy string) string {
	switch orderBy {
	case "last_seen_desc":
		return "last_seen DESC"
	case "count_desc":
		return "count DESC"
	default: // "priority_desc" or empty
		return "priority DESC, last_seen DESC"
	}
}

// scanCluster scans one row from the clusters table into a model.Cluster, it works with both pgx.Row and pgx.Rows.
func scanCluster(row pgx.Row) (model.Cluster, error) {
	var c model.Cluster
	var flagsJSON, servicesJSON, levelsJSON, examplesJSON string

	if err := row.Scan(
		&c.ID, &c.Fingerprint, &c.Template,
		&c.FirstSeen, &c.LastSeen, &c.Count, &c.Priority,
		&flagsJSON, &servicesJSON, &levelsJSON, &examplesJSON,
	); err != nil {
		return model.Cluster{}, err
	}

	if err := unmarshalStringSlice(flagsJSON, &c.AnomalyFlags); err != nil {
		return model.Cluster{}, fmt.Errorf("unmarshal anomaly_flags: %w", err)
	}
	if err := unmarshalStringSlice(servicesJSON, &c.Services); err != nil {
		return model.Cluster{}, fmt.Errorf("unmarshal services: %w", err)
	}
	if err := unmarshalStringSlice(examplesJSON, &c.ExamplesSample); err != nil {
		return model.Cluster{}, fmt.Errorf("unmarshal examples_sample: %w", err)
	}

	if levelsJSON != "" && levelsJSON != "null" {
		var raw map[string]int64
		if err := json.Unmarshal([]byte(levelsJSON), &raw); err != nil {
			return model.Cluster{}, fmt.Errorf("unmarshal levels_json: %w", err)
		}
		if len(raw) > 0 {
			c.Levels = make(map[model.Level]int64, len(raw))
			for k, v := range raw {
				lvl, err := strconv.Atoi(k)
				if err != nil {
					return model.Cluster{}, fmt.Errorf("invalid level key %q: %w", k, err)
				}
				c.Levels[model.Level(lvl)] = v
			}
		}
	}
	return c, nil
}

// servicesMatch returns true if cluster's services overlap with the filter list, or if the filter list is empty.
func servicesMatch(have, want []string) bool {
	if len(want) == 0 {
		return true
	}
	set := make(map[string]bool, len(want))
	for _, w := range want {
		set[w] = true
	}
	for _, h := range have {
		if set[h] {
			return true
		}
	}
	return false
}

func levelsMatch(have map[model.Level]int64, want []model.Level) bool {
	if len(want) == 0 {
		return true
	}
	for _, l := range want {
		if have[l] > 0 {
			return true
		}
	}
	return false
}

func unmarshalStringSlice(raw string, out *[]string) error {
	if raw == "" || raw == "null" {
		*out = []string{}
		return nil
	}
	return json.Unmarshal([]byte(raw), out)
}
