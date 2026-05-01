package analyze

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/Tragidra/logstruct/model"
)

// systemPrompt is the original instruction.
const systemPrompt = `You are an experienced site reliability engineer analyzing production logs. Your job is to interpret log clusters and give operators actionable guidance.

Rules:
- Be concrete. Reference specific fields, services, timestamps, or patterns from the input. Avoid generic advice.
- When uncertain, say so. Confidence calibration: 0.0-0.3 is a rough guess with little evidence, 0.4-0.6 is plausible given the data, 0.7-0.8 is well-supported by the logs, 0.9+ requires specific evidence (exact error code, named host, clear cause). Never inflate confidence.
- Suggested actions must be things an operator can do in the next 5 minutes: check a log, run a command, inspect a metric. No vague statements like "investigate further" or "monitor the situation."
- Every suggested action must be specific and start with a verb (Check, Run, Inspect, Verify, Tail, Restart, etc.).
- Severity: "info" for expected or benign behavior; "warning" for anomalies worth tracking; "critical" only when user-facing impact is confirmed or highly likely.
- If the log pattern looks benign (health checks, routine operations, debug noise), say so explicitly — don't invent drama.
- Never suggest disabling monitoring, logging, or alerting.
- Never claim root cause with certainty from logs alone. Always frame as a hypothesis: "likely", "possibly", "suggests".
- related_cluster_ids must only contain IDs from the NEIGHBORING CLUSTERS section — never invent IDs.

Output strictly valid JSON matching the provided schema. No markdown fences, no extra commentary.`

// userPromptTemplate mirrors prompts/cluster_analysis.md (User prompt).
const userPromptTemplate = `CLUSTER UNDER ANALYSIS

Cluster ID: {{.ClusterID}}
Template:   {{.Template}}
Total events seen: {{.CountTotal}}
Events in last window ({{.WindowDuration}}): {{.CountInWindow}}
Services involved: {{if .Services}}{{join .Services ", "}}{{else}}(unknown){{end}}
Level breakdown: {{levelBreakdown .LevelBreakdown}}
Time span: {{.TimeSpan}}
Scoring flags: {{if .Flags}}{{join .Flags ", "}}{{else}}(none){{end}}

EXAMPLE LOG LINES (up to 5)
{{range .Examples}}---
{{.}}
{{end}}
NEIGHBORING CLUSTERS IN SAME WINDOW
{{if .Neighbors}}{{range .Neighbors}}• id={{.ID}} priority={{.Priority}} template="{{.Template}}" events={{.CountInWindow}} services={{join .Services ", "}}
{{end}}{{else}}(none)
{{end}}
TASK
Produce a JSON object with exactly these fields:
- summary: 1-2 sentence plain-English description of what is happening and its impact
- severity: exactly one of "info" | "warning" | "critical"
- root_cause_hypothesis: most likely cause in one sentence, framed as a hypothesis
- suggested_actions: 1-5 specific, verb-first operator steps ordered by priority (each ≥ 10 characters)
- related_cluster_ids: IDs from the NEIGHBORING CLUSTERS section that are likely causally related; empty array if none
- confidence: float 0.0–1.0 — calibrate honestly based on evidence strength`

// PromptData carries the variables interpolated into the user prompt
type PromptData struct {
	ClusterID      string
	Template       string
	CountTotal     int64
	CountInWindow  int64
	WindowDuration string
	Services       []string
	LevelBreakdown map[model.Level]int64
	TimeSpan       string
	Flags          []string
	Examples       []string
	Neighbors      []NeighborCluster
}

// NeighborCluster is a sibling cluster shown to the LLM for context
type NeighborCluster struct {
	ID            string
	Template      string
	Priority      int
	CountInWindow int64
	Services      []string
}

var userPromptTpl = template.Must(template.New("user").Funcs(template.FuncMap{
	"join":           strings.Join,
	"levelBreakdown": formatLevelBreakdown,
}).Parse(userPromptTemplate))

func formatLevelBreakdown(lvls map[model.Level]int64) string {
	if len(lvls) == 0 {
		return "(none)"
	}
	order := []model.Level{
		model.LevelTrace, model.LevelDebug, model.LevelInfo,
		model.LevelWarn, model.LevelError, model.LevelFatal,
	}
	parts := make([]string, 0, len(lvls))
	for _, l := range order {
		if n, ok := lvls[l]; ok {
			parts = append(parts, fmt.Sprintf("%s=%d", l.String(), n))
		}
	}
	return strings.Join(parts, " ")
}

// RenderUserPrompt fills in the user-prompt template with data
func RenderUserPrompt(data PromptData) (string, error) {
	var buf bytes.Buffer
	if err := userPromptTpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("analyze: render user prompt: %w", err)
	}
	return buf.String(), nil
}

func SystemPrompt() string { return systemPrompt }
