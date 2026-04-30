package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Tragidra/logsense/internal/storage"
	"github.com/Tragidra/logsense/model"
)

// SaveEvent inserts a single normalized log event
func (s *Store) SaveEvent(ctx context.Context, e model.LogEvent, clusterID string) error {
	var fieldsJSON sql.NullString
	if len(e.Fields) > 0 {
		b, err := json.Marshal(e.Fields)
		if err != nil {
			return fmt.Errorf("save event: marshal fields: %w", err)
		}
		fieldsJSON = sql.NullString{String: string(b), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events
			(id, cluster_id, ts, level, service, message, fields_json, raw, source, trace_id)
		VALUES
			(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, clusterID, formatTime(e.Timestamp), int(e.Level),
		nullableString(e.Service), e.Message, fieldsJSON,
		nullableString(e.Raw), e.Source, nullableString(e.TraceID),
	)
	if err != nil {
		return fmt.Errorf("save event: %w", err)
	}
	return nil
}

// ListEventsByCluster returns events for a cluster ordered by ts DESC, default time range: last 1 hour, limit 200.
func (s *Store) ListEventsByCluster(ctx context.Context, clusterID string, filter storage.EventFilter) ([]model.LogEvent, error) {
	conds := []string{"cluster_id = ?"}
	args := []any{clusterID}

	from := filter.From
	if from == nil {
		t := time.Now().Add(-1 * time.Hour)
		from = &t
	}
	conds = append(conds, "ts >= ?")
	args = append(args, formatTime(*from))

	if filter.To != nil {
		conds = append(conds, "ts <= ?")
		args = append(args, formatTime(*filter.To))
	}

	if len(filter.Levels) > 0 {
		placeholders := make([]string, len(filter.Levels))
		for i, l := range filter.Levels {
			placeholders[i] = "?"
			args = append(args, int(l))
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

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var out []model.LogEvent
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("list events scan: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list events rows: %w", err)
	}
	return out, nil
}

func scanEvent(row scanner) (model.LogEvent, error) {
	var e model.LogEvent
	var clusterID, ts string
	var levelInt int
	var service, raw, traceID, fieldsJSON sql.NullString

	if err := row.Scan(
		&e.ID, &clusterID, &ts, &levelInt,
		&service, &e.Message, &fieldsJSON, &raw, &e.Source, &traceID,
	); err != nil {
		return model.LogEvent{}, err
	}

	e.Level = model.Level(levelInt)
	t, err := parseTime(ts)
	if err != nil {
		return model.LogEvent{}, fmt.Errorf("parse ts: %w", err)
	}
	e.Timestamp = t
	if service.Valid {
		e.Service = service.String
	}
	if raw.Valid {
		e.Raw = raw.String
	}
	if traceID.Valid {
		e.TraceID = traceID.String
	}
	if fieldsJSON.Valid && fieldsJSON.String != "" && fieldsJSON.String != "null" {
		if err := json.Unmarshal([]byte(fieldsJSON.String), &e.Fields); err != nil {
			return model.LogEvent{}, fmt.Errorf("unmarshal fields_json: %w", err)
		}
	}
	return e, nil
}
