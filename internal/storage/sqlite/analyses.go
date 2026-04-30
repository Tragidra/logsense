package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tragidra/loglens/model"
)

const analysisColumns = `
	id, cluster_id, window_start, window_end,
	summary, severity, root_cause, suggested_actions,
	related_cluster_ids, confidence, model_used,
	tokens_input, tokens_output, latency_ms, created_at`

// SaveAnalysis inserts an analysis; on conflict the existing row is updated
// so re-runs for the same window are idempotent.
func (s *Store) SaveAnalysis(ctx context.Context, a model.Analysis) error {
	actions := a.SuggestedActions
	if actions == nil {
		actions = []string{}
	}
	related := a.RelatedClusterIDs
	if related == nil {
		related = []string{}
	}

	actionsJSON, err := json.Marshal(actions)
	if err != nil {
		return fmt.Errorf("save analysis: marshal actions: %w", err)
	}
	relatedJSON, err := json.Marshal(related)
	if err != nil {
		return fmt.Errorf("save analysis: marshal related: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO analyses (
			id, cluster_id, window_start, window_end,
			summary, severity, root_cause, suggested_actions,
			related_cluster_ids, confidence, model_used,
			tokens_input, tokens_output, latency_ms, created_at
		) VALUES (
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?
		)
		ON CONFLICT (cluster_id, window_start, window_end) DO UPDATE SET
			summary             = excluded.summary,
			severity            = excluded.severity,
			root_cause          = excluded.root_cause,
			suggested_actions   = excluded.suggested_actions,
			related_cluster_ids = excluded.related_cluster_ids,
			confidence          = excluded.confidence,
			model_used          = excluded.model_used,
			tokens_input        = excluded.tokens_input,
			tokens_output       = excluded.tokens_output,
			latency_ms          = excluded.latency_ms`,
		a.ID, a.ClusterID, formatTime(a.WindowStart), formatTime(a.WindowEnd),
		a.Summary, int(a.Severity), nullableString(a.RootCauseHypothesis), string(actionsJSON),
		string(relatedJSON), a.Confidence, a.ModelUsed,
		a.TokensInput, a.TokensOutput, a.LatencyMs, formatTime(orNow(a.CreatedAt)),
	)
	if err != nil {
		return fmt.Errorf("save analysis: %w", err)
	}
	return nil
}

// LatestAnalysisForCluster returns the analysis with the greatest window_end for the given cluster,
// returns (nil, nil) if no analysis exists.
func (s *Store) LatestAnalysisForCluster(ctx context.Context, clusterID string) (*model.Analysis, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+analysisColumns+`
		 FROM analyses
		 WHERE cluster_id = ?
		 ORDER BY window_end DESC
		 LIMIT 1`, clusterID)

	a, err := scanAnalysis(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest analysis for cluster %s: %w", clusterID, err)
	}
	return &a, nil
}

// ListRecentAnalyses returns the most recently created analyses ordered by
// created_at DESC. limit <= 0 falls back to 20.
func (s *Store) ListRecentAnalyses(ctx context.Context, limit int) ([]model.Analysis, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+analysisColumns+`
		 FROM analyses
		 ORDER BY created_at DESC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent analyses: %w", err)
	}
	defer rows.Close()

	var out []model.Analysis
	for rows.Next() {
		a, err := scanAnalysis(rows)
		if err != nil {
			return nil, fmt.Errorf("list recent analyses scan: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list recent analyses rows: %w", err)
	}
	return out, nil
}

func scanAnalysis(row scanner) (model.Analysis, error) {
	var a model.Analysis
	var severityInt int
	var rootCause sql.NullString
	var windowStart, windowEnd, createdAt string
	var actionsJSON, relatedJSON string
	var tokensIn, tokensOut, latency sql.NullInt64
	var confidence sql.NullFloat64

	if err := row.Scan(
		&a.ID, &a.ClusterID, &windowStart, &windowEnd,
		&a.Summary, &severityInt, &rootCause, &actionsJSON,
		&relatedJSON, &confidence, &a.ModelUsed,
		&tokensIn, &tokensOut, &latency, &createdAt,
	); err != nil {
		return model.Analysis{}, err
	}

	a.Severity = model.Severity(severityInt)
	if rootCause.Valid {
		a.RootCauseHypothesis = rootCause.String
	}

	var err error
	if a.WindowStart, err = parseTime(windowStart); err != nil {
		return model.Analysis{}, fmt.Errorf("parse window_start: %w", err)
	}
	if a.WindowEnd, err = parseTime(windowEnd); err != nil {
		return model.Analysis{}, fmt.Errorf("parse window_end: %w", err)
	}
	if a.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.Analysis{}, fmt.Errorf("parse created_at: %w", err)
	}

	if err := unmarshalStringSlice(actionsJSON, &a.SuggestedActions); err != nil {
		return model.Analysis{}, fmt.Errorf("unmarshal suggested_actions: %w", err)
	}
	if err := unmarshalStringSlice(relatedJSON, &a.RelatedClusterIDs); err != nil {
		return model.Analysis{}, fmt.Errorf("unmarshal related_cluster_ids: %w", err)
	}

	if confidence.Valid {
		a.Confidence = float32(confidence.Float64)
	}
	if tokensIn.Valid {
		a.TokensInput = int(tokensIn.Int64)
	}
	if tokensOut.Valid {
		a.TokensOutput = int(tokensOut.Int64)
	}
	if latency.Valid {
		a.LatencyMs = int(latency.Int64)
	}
	return a, nil
}
