// Package normalize converts RawLog records into structured LogEvents.
package normalize

import (
	"regexp"
	"strings"
	"time"

	"github.com/Tragidra/logstruct/model"
)

// Normalizer converts a RawLog into a canonical LogEvent.
type Normalizer interface {
	Normalize(r model.RawLog) model.LogEvent
}

func New() Normalizer { return &defaultNormalizer{} }

type defaultNormalizer struct{}

const maxRawBytes = 64 * 1024

var (
	reNginx = regexp.MustCompile(
		`^(\S+) \S+ (\S+) \[([^\]]+)\] "(\S+) ([^"]*?) HTTP/[^"]*" (\d+) (\S+)`,
	)
	reSyslog = regexp.MustCompile(
		`^(?:<\d+>)?([A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+(\S+)\s+(\S+?)(?:\[(\d+)\])?:\s*(.*)$`,
	)
)

func (n *defaultNormalizer) Normalize(r model.RawLog) model.LogEvent {
	raw := r.Raw
	if len(raw) > maxRawBytes {
		raw = raw[:maxRawBytes]
	}

	e := model.LogEvent{
		ID:     model.NewID(),
		Raw:    raw,
		Source: r.Source,
	}

	trimmed := strings.TrimSpace(raw)

	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		if extractJSON(trimmed, &e, r.ReceivedAt) {
			if e.Level == model.LevelUnknown && e.Message != "" {
				e.Level = inferLevel(e.Message)
			}
			if e.Timestamp.IsZero() {
				e.Timestamp = r.ReceivedAt
			}
			e.Fingerprint = Fingerprint(e.Message)
			return e
		}
		// Syntactically {…} but not valid JSON — fall through.
	}

	// Nginx combined log format (on 04.2026)
	if tryNginx(trimmed, &e) {
		e.Fingerprint = Fingerprint(e.Message)
		return e
	}

	if trySyslog(trimmed, &e, r.ReceivedAt) {
		e.Fingerprint = Fingerprint(e.Message)
		return e
	}

	e.Message = raw
	e.Timestamp = r.ReceivedAt
	e.Level = inferLevel(raw)
	e.Fingerprint = Fingerprint(raw)
	return e
}

func tryNginx(raw string, e *model.LogEvent) bool {
	m := reNginx.FindStringSubmatch(raw)
	if m == nil {
		return false
	}
	ts, err := time.Parse("02/Jan/2006:15:04:05 -0700", m[3])
	if err != nil {
		return false
	}
	e.Timestamp = ts.UTC()
	e.Level = model.LevelInfo
	e.Message = m[4] + " " + m[5]
	e.Fields = map[string]any{
		"remote_addr": m[1],
		"user":        m[2],
		"status":      m[6],
		"body_bytes":  m[7],
	}
	return true
}

func trySyslog(raw string, e *model.LogEvent, fallback time.Time) bool {
	m := reSyslog.FindStringSubmatch(raw)
	if m == nil {
		return false
	}
	ts := parseSyslogTimestamp(m[1], fallback.Year())
	if ts.IsZero() {
		ts = fallback
	}
	e.Timestamp = ts
	e.Message = m[5]
	e.Level = inferLevel(m[5])
	e.Fields = map[string]any{
		"hostname": m[2],
		"tag":      m[3],
	}
	if m[4] != "" {
		e.Fields["pid"] = m[4]
	}
	return true
}
