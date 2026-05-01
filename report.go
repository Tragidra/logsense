package logstruct

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tragidra/logstruct/internal/analyze"
	"github.com/Tragidra/logstruct/internal/llm"
	"github.com/Tragidra/logstruct/model"
)

// ErrNotStarted is returned by Report and AnalyzeNow if the logstruct instance
// has not been started or has already been closed.
var ErrNotStarted = errors.New("logstruct: not started or already closed")

// ErrAIDisabled is returned when AnalyzeNow is called but the configured AI
// provider is unusable (e.g. missing credentials).
var ErrAIDisabled = errors.New("logstruct: AI provider not configured")

// Report ingests a runtime error as a synthetic log event. It is
// non-blocking: if the internal pipeline is full, the event is dropped and an
// internal counter is incremented (see Stats()).
//
// Report is a no-op if Inline mode is disabled in the config, if the instance
// is not running, or if err is nil.
//
// Fields may be nil. Nested values in fields are JSON-marshalled when stored.
func (ll *logstruct) Report(ctx context.Context, err error, fields Fields) {
	if err == nil || !ll.cfg.Inline.Enabled || !ll.running.Load() {
		return
	}

	raw := model.RawLog{
		Source:     "inline",
		SourceKind: "inline",
		Raw:        buildInlineLine(err, fields),
		ReceivedAt: time.Now().UTC(),
		Metadata:   map[string]string{"inline": "true"},
	}

	select {
	case ll.rawCh <- raw:
	case <-ctx.Done():
	default:
		ll.dropped.Add(1)
	}
}

// AnalyzeNow performs a one-shot LLM analysis of a single message, bypassing
// clustering entirely. It is synchronous and uses the caller's context for
// the timeout.
//
// This is expensive per call and lacks the cluster context that the regular
// pipeline uses — prefer file/inline ingestion + automatic analysis. Use
// AnalyzeNow only when you need a recommendation right now for a specific
// error and don't want to wait for clustering to converge.
//
// The returned Analysis has ClusterID = "" and is not persisted.
func (ll *logstruct) AnalyzeNow(ctx context.Context, message string, fields Fields) (*model.Analysis, error) {
	if ll.provider == nil {
		return nil, ErrAIDisabled
	}
	if strings.TrimSpace(message) == "" {
		return nil, errors.New("logstruct: AnalyzeNow: empty message")
	}

	prompt := buildDirectPrompt(message, fields)
	req := llm.Request{
		Messages: []llm.Message{
			{Role: "system", Content: analyze.SystemPrompt()},
			{Role: "user", Content: prompt},
		},
		JSONSchema:  analyze.ClusterAnalysisSchema,
		MaxTokens:   ll.cfg.AI.MaxTokens,
		Temperature: float32(ll.cfg.AI.Temperature),
	}

	start := time.Now()
	resp, err := ll.provider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("logstruct: AnalyzeNow: %w", err)
	}

	parsed, err := analyze.ParseAnalysisJSON(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("logstruct: AnalyzeNow: parse response: %w", err)
	}

	now := time.Now().UTC()
	return &model.Analysis{
		ID:                  model.NewID(),
		ClusterID:           "",
		WindowStart:         start,
		WindowEnd:           now,
		Summary:             parsed.Summary,
		Severity:            model.ParseSeverity(parsed.Severity),
		RootCauseHypothesis: parsed.RootCauseHypothesis,
		SuggestedActions:    parsed.SuggestedActions,
		RelatedClusterIDs:   parsed.RelatedClusterIDs,
		Confidence:          parsed.Confidence,
		ModelUsed:           resp.Model,
		TokensInput:         resp.InputTokens,
		TokensOutput:        resp.OutputTokens,
		LatencyMs:           resp.LatencyMs,
		CreatedAt:           now,
	}, nil
}

// RecentRecommendations returns the most recent stored Analyses across all
// clusters, ordered newest-first. limit <= 0 falls back to 20.
func (ll *logstruct) RecentRecommendations(ctx context.Context, limit int) ([]model.Analysis, error) {
	return ll.repo.ListRecentAnalyses(ctx, limit)
}

// buildInlineLine renders an inline Report() into a synthetic log line that
// the normalizer can parse. We emit a flat JSON object so the JSON parser path
// catches it cleanly and fields are preserved as structured data.
func buildInlineLine(err error, fields Fields) string {
	payload := map[string]any{
		"level":     "error",
		"message":   err.Error(),
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"source":    "inline",
	}
	for k, v := range fields {
		if _, exists := payload[k]; exists {
			continue
		}
		payload[k] = v
	}
	b, mErr := json.Marshal(payload)
	if mErr != nil {
		return err.Error()
	}
	return string(b)
}

// buildDirectPrompt renders a minimal prompt for AnalyzeNow. No cluster
// context is available, so we lean on the message + fields.
func buildDirectPrompt(message string, fields Fields) string {
	var b strings.Builder
	b.WriteString("DIRECT ERROR REPORT (no cluster context — single occurrence)\n\n")
	b.WriteString("Message: ")
	b.WriteString(message)
	b.WriteString("\n")
	if len(fields) > 0 {
		b.WriteString("\nFields:\n")
		fieldsJSON, err := json.MarshalIndent(fields, "", "  ")
		if err == nil {
			b.Write(fieldsJSON)
			b.WriteString("\n")
		}
	}
	b.WriteString(`
TASK
Analyze this single error and output a JSON object with the following fields:
- summary: 1-2 sentence plain-English summary
- severity: "info" | "warning" | "critical"
- root_cause_hypothesis: most likely cause in one sentence (it's a hypothesis — say so)
- suggested_actions: 1-5 concrete 5-minute checks or fixes; fewer is fine if you cannot suggest more
- related_cluster_ids: leave empty (no cluster context available)
- confidence: 0..1 — be honest, single-occurrence reports rarely warrant > 0.5`)
	return b.String()
}
