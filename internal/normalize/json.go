package normalize

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Tragidra/loglens/model"
)

type fieldAlias struct {
	target  string
	aliases []string
}

// fieldAliasMap defines extraction order and alias priority, earlier aliases win when a JSON contains multiple keys.
var fieldAliasMap = []fieldAlias{
	{"Timestamp", []string{"timestamp", "ts", "time", "@timestamp", "datetime"}},
	{"Level", []string{"level", "severity", "lvl", "log.level"}},
	{"Message", []string{"msg", "message", "log", "text"}},
	{"Service", []string{"service", "app", "service_name", "component"}},
	{"TraceID", []string{"trace_id", "traceId", "trace.id", "trace_context.trace_id"}},
	{"SpanID", []string{"span_id", "spanId", "span.id"}},
}

// extractJSON attempts to parse raw as a JSON object and populate e.
func extractJSON(raw string, e *model.LogEvent, fallback time.Time) bool {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return false
	}

	consumed := make(map[string]bool, len(fieldAliasMap)*3)

	for _, fa := range fieldAliasMap {
		for _, alias := range fa.aliases {
			v, ok := m[alias]
			if !ok {
				continue
			}
			consumed[alias] = true
			switch fa.target {
			case "Timestamp":
				e.Timestamp = parseTimestamp(v, fallback)
			case "Level":
				if s, ok := v.(string); ok {
					e.Level = normalizeLevel(s)
				}
			case "Message":
				e.Message = fmt.Sprint(v)
			case "Service":
				e.Service = fmt.Sprint(v)
			case "TraceID":
				e.TraceID = fmt.Sprint(v)
			case "SpanID":
				e.SpanID = fmt.Sprint(v)
			}
			break // first alias match wins
		}
	}

	// Remaining fields, drop internal keys prefixed with "_".
	var fields map[string]any
	for k, v := range m {
		if consumed[k] || strings.HasPrefix(k, "_") {
			continue
		}
		if fields == nil {
			fields = make(map[string]any)
		}
		fields[k] = v
	}
	e.Fields = fields

	return true
}
