package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Tragidra/logstruct/model"
)

const analysisColumns = `
	id, cluster_id, window_start, window_end,
	summary, severity, root_cause, suggested_actions,
	related_cluster_ids, confidence, model_used,
	tokens_input, tokens_output, latency_ms, created_at`

// SaveAnalysis inserts an analysis; the existing row is updated so re-runs for the same window are idempotent.
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

	_, err = s.pool.Exec(ctx, `
		INSERT INTO analyses (
			id, cluster_id, window_start, window_end,
			summary, severity, root_cause, suggested_actions,
			related_cluster_ids, confidence, model_used,
			tokens_input, tokens_output, latency_ms
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8,
			$9, $10, $11,
			$12, $13, $14
		)
		ON CONFLICT (cluster_id, window_start, window_end) DO UPDATE SET
			summary             = EXCLUDED.summary,
			severity            = EXCLUDED.severity,
			root_cause          = EXCLUDED.root_cause,
			suggested_actions   = EXCLUDED.suggested_actions,
			related_cluster_ids = EXCLUDED.related_cluster_ids,
			confidence          = EXCLUDED.confidence,
			model_used          = EXCLUDED.model_used,
			tokens_input        = EXCLUDED.tokens_input,
			tokens_output       = EXCLUDED.tokens_output,
			latency_ms          = EXCLUDED.latency_ms`,
		a.ID, a.ClusterID, a.WindowStart, a.WindowEnd,
		a.Summary, int(a.Severity), nullableString(a.RootCauseHypothesis), string(actionsJSON),
		string(relatedJSON), a.Confidence, a.ModelUsed,
		a.TokensInput, a.TokensOutput, a.LatencyMs,
	)
	if err != nil {
		return fmt.Errorf("save analysis: %w", err)
	}
	return nil
}

// ListRecentAnalyses returns the most recently created analyses ordered by created_at DESC
func (s *Store) ListRecentAnalyses(ctx context.Context, limit int) ([]model.Analysis, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+analysisColumns+`
		 FROM analyses
		 ORDER BY created_at DESC
		 LIMIT $1`, limit)
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

// LatestAnalysisForCluster returns the analysis with the greatest window_end for the given cluster
func (s *Store) LatestAnalysisForCluster(ctx context.Context, clusterID string) (*model.Analysis, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+analysisColumns+`
		 FROM analyses
		 WHERE cluster_id = $1
		 ORDER BY window_end DESC
		 LIMIT 1`, clusterID)

	a, err := scanAnalysis(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest analysis for cluster %s: %w", clusterID, err)
	}
	return &a, nil
}

func scanAnalysis(row pgx.Row) (model.Analysis, error) {
	var a model.Analysis
	var severityInt int
	var rootCause *string
	var actionsJSON, relatedJSON string

	if err := row.Scan(
		&a.ID, &a.ClusterID, &a.WindowStart, &a.WindowEnd,
		&a.Summary, &severityInt, &rootCause, &actionsJSON,
		&relatedJSON, &a.Confidence, &a.ModelUsed,
		&a.TokensInput, &a.TokensOutput, &a.LatencyMs, &a.CreatedAt,
	); err != nil {
		return model.Analysis{}, err
	}

	a.Severity = model.Severity(severityInt)
	if rootCause != nil {
		a.RootCauseHypothesis = *rootCause
	}
	if err := unmarshalStringSlice(actionsJSON, &a.SuggestedActions); err != nil {
		return model.Analysis{}, fmt.Errorf("unmarshal suggested_actions: %w", err)
	}
	if err := unmarshalStringSlice(relatedJSON, &a.RelatedClusterIDs); err != nil {
		return model.Analysis{}, fmt.Errorf("unmarshal related_cluster_ids: %w", err)
	}
	return a, nil
}

// nullableString returns nil for an empty string so NULLs cleanly in columns that are optional
// (root_cause, event.service, event.trace_id).
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
