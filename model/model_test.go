package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLevel_String(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelUnknown, "unknown"},
		{LevelTrace, "trace"},
		{LevelDebug, "debug"},
		{LevelInfo, "info"},
		{LevelWarn, "warn"},
		{LevelError, "error"},
		{LevelFatal, "fatal"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.level.String())
		})
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  Level
	}{
		{"trace", LevelTrace},
		{"TRACE", LevelTrace},
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"dbg", LevelDebug},
		{"verbose", LevelDebug},
		{"verb", LevelDebug},
		{"v", LevelDebug},
		{"info", LevelInfo},
		{"INFO", LevelInfo},
		{"information", LevelInfo},
		{"inf", LevelInfo},
		{"i", LevelInfo},
		{"warn", LevelWarn},
		{"WARN", LevelWarn},
		{"warning", LevelWarn},
		{"wrn", LevelWarn},
		{"w", LevelWarn},
		{"error", LevelError},
		{"ERROR", LevelError},
		{"err", LevelError},
		{"e", LevelError},
		{"fatal", LevelFatal},
		{"FATAL", LevelFatal},
		{"crit", LevelFatal},
		{"critical", LevelFatal},
		{"alert", LevelFatal},
		{"emerg", LevelFatal},
		{"emergency", LevelFatal},
		{"panic", LevelFatal},
		{"", LevelUnknown},
		{"bogus", LevelUnknown},
		{"  info  ", LevelInfo}, // whitespace trimmed
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseLevel(tt.input))
		})
	}
}

func TestParseLevel_RoundTrip(t *testing.T) {
	levels := []Level{LevelTrace, LevelDebug, LevelInfo, LevelWarn, LevelError, LevelFatal}
	for _, l := range levels {
		t.Run(l.String(), func(t *testing.T) {
			assert.Equal(t, l, ParseLevel(l.String()))
		})
	}
}

func TestSeverity_String(t *testing.T) {
	tests := []struct {
		sev  Severity
		want string
	}{
		{SeverityUnknown, "unknown"},
		{SeverityInfo, "info"},
		{SeverityWarning, "warning"},
		{SeverityCritical, "critical"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.sev.String())
		})
	}
}

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		input string
		want  Severity
	}{
		{"info", SeverityInfo},
		{"warning", SeverityWarning},
		{"warn", SeverityWarning},
		{"critical", SeverityCritical},
		{"crit", SeverityCritical},
		{"unknown", SeverityUnknown},
		{"bogus", SeverityUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseSeverity(tt.input))
		})
	}
}

func TestNewID(t *testing.T) {
	id := NewID()
	require.NotEmpty(t, id)
	assert.Len(t, id, 26)
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	for _, ch := range id {
		assert.True(t, strings.ContainsRune(alphabet, ch), "unexpected char %q in ULID %q", ch, id)
	}
}

func TestNewID_Unique(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id := NewID()
		assert.False(t, seen[id], "duplicate ULID: %s", id)
		seen[id] = true
	}
}

func TestNewID_Monotonic(t *testing.T) {
	prev := NewID()
	for i := 0; i < 100; i++ {
		curr := NewID()
		assert.GreaterOrEqual(t, curr, prev, "ULID ordering violated")
		prev = curr
	}
}
