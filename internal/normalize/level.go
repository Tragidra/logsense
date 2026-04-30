package normalize

import (
	"strings"

	"github.com/Tragidra/loglens/model"
)

// normalizeLevel maps a raw level string to a model.Level.
func normalizeLevel(s string) model.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "TRACE":
		return model.LevelTrace
	case "DEBUG", "DBG":
		return model.LevelDebug
	case "INFO", "INFORMATION", "NOTICE":
		return model.LevelInfo
	case "WARN", "WARNING":
		return model.LevelWarn
	case "ERROR", "ERR", "SEVERE":
		return model.LevelError
	case "FATAL", "CRITICAL", "CRIT", "EMERG", "EMERGENCY", "PANIC":
		return model.LevelFatal
	}
	return model.LevelUnknown
}

// inferLevel scans msg for severity keywords when an explicit level is absent.
func inferLevel(msg string) model.Level {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "panic") ||
		strings.Contains(lower, "fatal") ||
		strings.Contains(lower, "critical"):
		return model.LevelFatal
	case strings.Contains(lower, "error") ||
		strings.Contains(lower, "err ") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "failure"):
		return model.LevelError
	case strings.Contains(lower, "warn"):
		return model.LevelWarn
	default:
		return model.LevelInfo
	}
}
