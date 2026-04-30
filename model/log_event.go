package model

import (
	"strings"
	"time"
)

// RawLog is the unprocessed record emitted by an ingest source, it is discarded after normalization.
type RawLog struct {
	Source     string            // source name from config
	SourceKind string            // "file" | "kafka" | "http" | "syslog"
	Raw        string            // exact bytes as received
	ReceivedAt time.Time         // when we received it
	Metadata   map[string]string // source-specific hints (topic, file path, etc.)
}

// LogEvent is the canonical normalized form produced by the normalizer.
type LogEvent struct {
	ID          string         // ID, time-ordered
	Timestamp   time.Time      // event time (parsed from log, else ReceivedAt)
	Level       Level          // normalized severity level
	Message     string         // message
	Service     string         // service that produced this event
	TraceID     string         // distributed trace ID if extracted
	SpanID      string         // span ID if extracted
	Fields      map[string]any // additional parsed key-value pairs
	Raw         string         // original truncated line
	Source      string         // source name
	Fingerprint string         // hash of tokenized template for clustering
	ParseError  string         // non-empty if parsing was incomplete
}

// Level is a normalized log severity level.
type Level int

const (
	LevelUnknown Level = iota
	LevelTrace
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// String returns the string representation of the level.
func (l Level) String() string {
	switch l {
	case LevelTrace:
		return "trace"
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	case LevelFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// ParseLevel converts a level string to a Level.
// Returns LevelUnknown for unrecognised values.
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return LevelTrace
	case "debug", "dbg", "verbose", "verb", "v":
		return LevelDebug
	case "info", "information", "inf", "i":
		return LevelInfo
	case "warn", "warning", "wrn", "w":
		return LevelWarn
	case "error", "err", "e":
		return LevelError
	case "fatal", "crit", "critical", "alert", "emerg", "emergency", "panic":
		return LevelFatal
	default:
		return LevelUnknown
	}
}
