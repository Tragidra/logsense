package sqlite

import (
	"errors"
	"time"
)

// timeFormat is the canonical wire format for timestamps stored as TEXT.
const timeFormat = time.RFC3339Nano

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(timeFormat)
}

// parseTime accepts the canonical RFC3339Nano format and a few common SQLite-shaped variants
// ("YYYY-MM-DD HH:MM:SS[.fff]" with no timezone).
func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05.999999999Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, errors.New("sqlite: unrecognised time format: " + s)
}

// nullableString returns a *string for empty-as-NULL columns. Wrapping in *string lets database/sql encode SQL NULL
// when the value is nil.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func orNow(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t
}
