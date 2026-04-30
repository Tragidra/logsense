package normalize

import (
	"strconv"
	"time"
)

var tsLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"02/Jan/2006:15:04:05 -0700",
}

// parseTimestamp converts a JSON timestamp value (string or float64 unix epoch) to time.Time
func parseTimestamp(v any, fallback time.Time) time.Time {
	switch tv := v.(type) {
	case string:
		for _, layout := range tsLayouts {
			if t, err := time.Parse(layout, tv); err == nil {
				return t.UTC()
			}
		}
	case float64:
		if tv > 1e12 {
			// Unix milliseconds (13+ digits)
			return time.UnixMilli(int64(tv)).UTC()
		}
		sec := int64(tv)
		nsec := int64((tv - float64(sec)) * 1e9)
		return time.Unix(sec, nsec).UTC()
	}
	return fallback
}

// parseSyslogTimestamp parses an RFC3164-style timestamp ("Apr 30 00:00:00"), returns zero time on failure.
func parseSyslogTimestamp(s string, year int) time.Time {
	prefix := strconv.Itoa(year) + " "
	for _, layout := range []string{
		"2006 Jan _2 15:04:05",
		"2006 Jan 2 15:04:05",
		"2006 Jan 02 15:04:05",
	} {
		if t, err := time.Parse(layout, prefix+s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
