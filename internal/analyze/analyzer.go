package analyze

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Tragidra/logstruct/internal/config"
	"github.com/Tragidra/logstruct/internal/llm"
	"github.com/Tragidra/logstruct/internal/storage"
	"github.com/Tragidra/logstruct/model"
)

// Analyzer turns clusters into LLM-generated Analyses-module
type Analyzer interface {
	Analyze(ctx context.Context, clusterID string) (*model.Analysis, error)
}

type analyzer struct {
	cfg      config.AnalyzeConfig
	provider llm.Provider
	repo     storage.Repository
	cache    *cache
	logger   *slog.Logger
}

// New constructs an Analyzer.
func New(cfg config.AnalyzeConfig, provider llm.Provider, repo storage.Repository, logger *slog.Logger) Analyzer {
	ttl := cfg.CacheTTL.D()
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &analyzer{
		cfg:      cfg,
		provider: provider,
		repo:     repo,
		cache:    newCache(256, ttl),
		logger:   logger,
	}
}

// Analyze returns the latest Analysis for clusterID, computing one via LLM if the cache is cold or stale.
func (a *analyzer) Analyze(ctx context.Context, clusterID string) (*model.Analysis, error) {
	cluster, err := a.repo.GetCluster(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("analyze: get cluster: %w", err)
	}
	clusterPtr := &cluster

	now := time.Now().UTC()
	windowDur := a.cfg.Window.D()
	if windowDur <= 0 {
		windowDur = 5 * time.Minute
	}
	windowStart := now.Add(-windowDur).Truncate(5 * time.Minute)
	windowEnd := windowStart.Add(windowDur)

	if cached := a.cache.Get(cluster.Fingerprint, windowStart); cached != nil {
		return cached, nil
	}

	prompt, err := RenderUserPrompt(PromptData{
		ClusterID:      clusterID,
		Template:       cluster.Template,
		CountTotal:     cluster.Count,
		CountInWindow:  cluster.Count,
		WindowDuration: windowDur.String(),
		Services:       cluster.Services,
		LevelBreakdown: cluster.Levels,
		TimeSpan:       fmt.Sprintf("%s to %s", cluster.FirstSeen.Format(time.RFC3339), cluster.LastSeen.Format(time.RFC3339)),
		Flags:          cluster.AnomalyFlags,
		Examples:       cluster.ExamplesSample,
	})
	if err != nil {
		return nil, err
	}

	req := llm.Request{
		Messages: []llm.Message{
			{Role: "system", Content: SystemPrompt()},
			{Role: "user", Content: prompt},
		},
		JSONSchema:  ClusterAnalysisSchema,
		MaxTokens:   1500,
		Temperature: 0.2,
	}

	resp, err := a.provider.Complete(ctx, req)
	if err != nil {
		a.logger.Warn("analyze: llm call failed", "cluster_id", clusterID, "err", err)
		return a.fallback(clusterPtr, windowStart, windowEnd, "Analysis failed, please retry."), nil
	}

	parsed, err := ParseAnalysisJSON(resp.Content)
	if err != nil {
		a.logger.Warn("analyze: invalid llm json", "cluster_id", clusterID, "err", err, "snippet", snippet(resp.Content, 200))
		return a.fallback(clusterPtr, windowStart, windowEnd, "Analysis returned malformed JSON, please retry."), nil
	}

	an := &model.Analysis{
		ID:                  model.NewID(),
		ClusterID:           clusterID,
		WindowStart:         windowStart,
		WindowEnd:           windowEnd,
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
		CreatedAt:           time.Now().UTC(),
	}

	if err := a.repo.SaveAnalysis(ctx, *an); err != nil {
		a.logger.Error("analyze: save analysis failed", "cluster_id", clusterID, "err", err)
	}

	a.cache.Set(cluster.Fingerprint, windowStart, an)
	return an, nil
}

func (a *analyzer) fallback(cluster *model.Cluster, ws, we time.Time, summary string) *model.Analysis {
	return &model.Analysis{
		ID:          model.NewID(),
		ClusterID:   cluster.ID,
		WindowStart: ws,
		WindowEnd:   we,
		Summary:     summary,
		Severity:    model.SeverityUnknown,
		Confidence:  0,
		CreatedAt:   time.Now().UTC(),
	}
}

// RawAnalysis matches the JSON schema returned by the LLM
type RawAnalysis struct {
	Summary             string   `json:"summary"`
	Severity            string   `json:"severity"`
	RootCauseHypothesis string   `json:"root_cause_hypothesis"`
	SuggestedActions    []string `json:"suggested_actions"`
	RelatedClusterIDs   []string `json:"related_cluster_ids"`
	Confidence          float32  `json:"confidence"`
}

// ParseAnalysisJSON decodes and validates an LLM JSON response
func ParseAnalysisJSON(content string) (*RawAnalysis, error) {
	var out RawAnalysis
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if out.Summary == "" || out.Severity == "" || out.RootCauseHypothesis == "" {
		return nil, errors.New("missing required fields")
	}
	if err := validateAnalysis(&out); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	if out.SuggestedActions == nil {
		out.SuggestedActions = []string{}
	}
	if out.RelatedClusterIDs == nil {
		out.RelatedClusterIDs = []string{}
	}
	return &out, nil
}

func validateAnalysis(a *RawAnalysis) error {
	switch a.Severity {
	case "info", "warning", "critical":
	default:
		return fmt.Errorf("invalid severity %q: must be info|warning|critical", a.Severity)
	}
	if a.Confidence < 0 || a.Confidence > 1 {
		return fmt.Errorf("confidence %g out of range [0,1]", a.Confidence)
	}
	if len(strings.TrimSpace(a.Summary)) < 10 {
		return fmt.Errorf("summary too short (%d chars)", len(strings.TrimSpace(a.Summary)))
	}
	if len(a.SuggestedActions) == 0 {
		return errors.New("suggested_actions must not be empty")
	}
	for i, action := range a.SuggestedActions {
		if len(strings.TrimSpace(action)) < 10 {
			return fmt.Errorf("suggested_actions[%d] too short: %q", i, action)
		}
	}
	return nil
}

func snippet(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
