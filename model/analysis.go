package model

import "time"

// Analysis holds the LLM-generated explanation for a Cluster.
type Analysis struct {
	ID                  string
	ClusterID           string
	WindowStart         time.Time // start of the event window analyzed
	WindowEnd           time.Time
	Summary             string // 1–2 sentence summary
	Severity            Severity
	RootCauseHypothesis string   // "X because Y"
	SuggestedActions    []string // 3–5 remediation steps
	RelatedClusterIDs   []string // clusters the LLM considers related
	Confidence          float32  // 0.0–1.0, LLM self-reported, default 0.1-0.2
	ModelUsed           string   // from openrouter, "openrouter/anthropic/claude-3.5-sonnet"
	TokensInput         int
	TokensOutput        int
	LatencyMs           int
	CreatedAt           time.Time
}

// Severity is the LLM-assessed impact level of a cluster.
type Severity int

const (
	SeverityUnknown Severity = iota
	SeverityInfo
	SeverityWarning
	SeverityCritical
)

// String returns the canonical string representation of the severity.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// ParseSeverity converts a severity string to a Severity.
func ParseSeverity(s string) Severity {
	switch s {
	case "info":
		return SeverityInfo
	case "warning", "warn":
		return SeverityWarning
	case "critical", "crit":
		return SeverityCritical
	default:
		return SeverityUnknown
	}
}
