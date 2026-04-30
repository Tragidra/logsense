package model

import "time"

// Cluster groups LogEvents that share the same log template.
type Cluster struct {
	ID             string
	Fingerprint    string          // identifier from template
	Template       string          // "payment gateway timeout"
	TemplateTokens []string        // tokenized form used for matching
	Services       []string        // services observed emitting this template
	Levels         map[Level]int64 // event count per severity level
	Count          int64           // total events matched to this cluster
	FirstSeen      time.Time
	LastSeen       time.Time
	ExamplesSample []string // reservoir-sampled raw examples (up to 5)
	Priority       int      // 0–100, updated by scorer
	AnomalyFlags   []string
	LatestAnalysis *Analysis // most recent AI analysis (nil/none)
}
