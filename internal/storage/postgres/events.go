package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Tragidra/logsense/internal/storage"
	"github.com/Tragidra/logsense/model"
)

// SaveEvent inserts a single log event
func (s *Store) SaveEvent(ctx context.Context, e model.LogEvent, clusterID string) error {
	var fieldsJSON string
	if len(e.Fields) > 0 {
		raw, err := json.Marshal(e.Fields)
		if err != nil {
			return fmt.Errorf("save event: marshal fields: %w", err)
		}
		fieldsJSON = string(raw)
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO events
			(id, cluster_id, ts, level, service, message, fields_json, raw, source, trace_id)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		e.ID, clusterID, e.Timestamp, int(e.Level),
		nullableString(e.Service), e.Message, nullableString(fieldsJSON),
		nullableString(e.Raw), e.Source, nullableString(e.TraceID),
	)
	if err != nil {
		return fmt.Errorf("save event: %w", err)
	}
	return nil
}

// ListEventsByCluster returns events for a cluster ordered by ts DESC, default time range: last 1 hour, limit: 200
func (s *Store) ListEventsByCluster(ctx context.Context, clusterID string, filter storage.EventFilter) ([]model.LogEvent, error) {
	conds := []string{"cluster_id = $1"}
	args := []any{clusterID}
	n := 2

	from := filter.From
	if from == nil {
		t := time.Now().Add(-1 * time.Hour)
		from = &t
	}
	conds = append(conds, fmt.Sprintf("ts >= $%d", n))
	args = append(args, *from)
	n++

	if filter.To != nil {
		conds = append(conds, fmt.Sprintf("ts <= $%d", n))
		args = append(args, *filter.To)
		n++
	}

	if len(filter.Levels) > 0 {
		placeholders := make([]string, len(filter.Levels))
		for i, l := range filter.Levels {
			placeholders[i] = fmt.Sprintf("$%d", n)
			args = append(args, int(l))
			n++
		}
		conds = append(conds, "level IN ("+strings.Join(placeholders, ", ")+")")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}

	query := fmt.Sprintf(`
		SELECT id, cluster_id, ts, level, service, message, fields_json, raw, source, trace_id
		FROM events
		WHERE %s
		ORDER BY ts DESC
		LIMIT %d OFFSET %d`,
		strings.Join(conds, " AND "), limit, filter.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var events []model.LogEvent
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("list events scan: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list events rows: %w", err)
	}
	return events, nil
}

func scanEvent(row pgx.Row) (model.LogEvent, error) {
	var e model.LogEvent
	var clusterID string // not part of LogEvent; scanned and discarded
	var levelInt int
	var service, fieldsJSON, raw, traceID *string

	if err := row.Scan(
		&e.ID, &clusterID, &e.Timestamp, &levelInt,
		&service, &e.Message, &fieldsJSON, &raw, &e.Source, &traceID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.LogEvent{}, storage.ErrNotFound
		}
		return model.LogEvent{}, err
	}

	e.Level = model.Level(levelInt)
	if service != nil {
		e.Service = *service
	}
	if raw != nil {
		e.Raw = *raw
	}
	if traceID != nil {
		e.TraceID = *traceID
	}

	if fieldsJSON != nil && *fieldsJSON != "" && *fieldsJSON != "null" {
		if err := json.Unmarshal([]byte(*fieldsJSON), &e.Fields); err != nil {
			return model.LogEvent{}, fmt.Errorf("unmarshal fields_json: %w", err)
		}
	}
	return e, nil
}
