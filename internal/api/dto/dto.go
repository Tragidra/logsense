package dto

import "time"

// ClusterDTO is the stable JSON shape for a cluster sent to the frontend
type ClusterDTO struct {
	ID           string           `json:"id"`
	Template     string           `json:"template"`
	Count        int64            `json:"count"`
	Priority     int              `json:"priority"`
	AnomalyFlags []string         `json:"anomaly_flags"`
	Services     []string         `json:"services"`
	Levels       map[string]int64 `json:"levels"`
	FirstSeen    time.Time        `json:"first_seen"`
	LastSeen     time.Time        `json:"last_seen"`
	Examples     []string         `json:"examples"`
	Analysis     *AnalysisDTO     `json:"analysis,omitempty"`
}

// AnalysisDTO is the stable JSON shape for an analysis.
type AnalysisDTO struct {
	Summary             string    `json:"summary"`
	Severity            string    `json:"severity"`
	RootCauseHypothesis string    `json:"root_cause_hypothesis"`
	SuggestedActions    []string  `json:"suggested_actions"`
	Confidence          float32   `json:"confidence"`
	ModelUsed           string    `json:"model_used"`
	CreatedAt           time.Time `json:"created_at"`
}

// LogEventDTO is the stable JSON shape for a log event.
type LogEventDTO struct {
	ID        string    `json:"id"`
	Message   string    `json:"message"`
	Level     string    `json:"level"`
	Service   string    `json:"service"`
	Timestamp time.Time `json:"timestamp"`
	Raw       string    `json:"raw,omitempty"`
}

// ListClustersResponse wraps a paginated list of clusters.
type ListClustersResponse struct {
	Items []ClusterDTO `json:"items"`
	Total int64        `json:"total"`
}

// ListEventsResponse wraps a paginated list of events.
type ListEventsResponse struct {
	Items []LogEventDTO `json:"items"`
}

// AnalyzeTriggerResponse is returned by POST /api/clusters/{id}/analyze.
type AnalyzeTriggerResponse struct {
	Analysis *AnalysisDTO `json:"analysis"`
}
