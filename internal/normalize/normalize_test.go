package normalize_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tragidra/loglens/internal/normalize"
	"github.com/Tragidra/loglens/model"
)

var fixedReceivedAt = time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

type goldenEvent struct {
	Level       string         `json:"level"`
	Message     string         `json:"message"`
	Service     string         `json:"service,omitempty"`
	TraceID     string         `json:"trace_id,omitempty"`
	SpanID      string         `json:"span_id,omitempty"`
	Timestamp   string         `json:"timestamp"`
	Fingerprint string         `json:"fingerprint"`
	Fields      map[string]any `json:"fields,omitempty"`
	ParseError  string         `json:"parse_error,omitempty"`
	Source      string         `json:"source,omitempty"`
}

func eventToGolden(e model.LogEvent) goldenEvent {
	return goldenEvent{
		Level:       e.Level.String(),
		Message:     e.Message,
		Service:     e.Service,
		TraceID:     e.TraceID,
		SpanID:      e.SpanID,
		Timestamp:   e.Timestamp.UTC().Format(time.RFC3339Nano),
		Fingerprint: e.Fingerprint,
		Fields:      e.Fields,
		ParseError:  e.ParseError,
		Source:      e.Source,
	}
}

func TestGolden(t *testing.T) {
	n := normalize.New()
	update := os.Getenv("UPDATE_GOLDEN") == "1"

	inputs, err := filepath.Glob("testdata/inputs/*.txt")
	require.NoError(t, err)
	require.NotEmpty(t, inputs, "no input fixtures found in testdata/inputs/")

	for _, inputPath := range inputs {
		inputPath := inputPath
		name := strings.TrimSuffix(filepath.Base(inputPath), ".txt")

		t.Run(name, func(t *testing.T) {
			rawBytes, err := os.ReadFile(inputPath)
			require.NoError(t, err)

			r := model.RawLog{
				Raw:        strings.TrimRight(string(rawBytes), "\n\r"),
				Source:     "test",
				ReceivedAt: fixedReceivedAt,
			}

			got := eventToGolden(n.Normalize(r))

			expectedPath := filepath.Join("testdata", "expected", name+".json")

			if update {
				data, err := json.MarshalIndent(got, "", "  ")
				require.NoError(t, err)
				require.NoError(t, os.MkdirAll(filepath.Dir(expectedPath), 0o755))
				require.NoError(t, os.WriteFile(expectedPath, append(data, '\n'), 0o644))
				t.Logf("updated %s", expectedPath)
				return
			}

			expectedBytes, err := os.ReadFile(expectedPath)
			require.NoError(t, err,
				"golden file missing — run: UPDATE_GOLDEN=1 go test ./internal/normalize/...")

			var want goldenEvent
			require.NoError(t, json.Unmarshal(expectedBytes, &want))

			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)
			require.JSONEq(t, string(wantJSON), string(gotJSON))
		})
	}
}

// Unit tests
func TestNormalize_TruncatesLongLine(t *testing.T) {
	n := normalize.New()
	long := strings.Repeat("x", 100*1024)
	e := n.Normalize(model.RawLog{Raw: long, Source: "test", ReceivedAt: time.Now()})
	assert.Len(t, e.Raw, 64*1024)
}

func TestFingerprint_SemanticEquality(t *testing.T) {
	fp1 := normalize.Fingerprint("user alice failed login from 1.2.3.4")
	fp2 := normalize.Fingerprint("user alice failed login from 5.6.7.8")
	assert.Equal(t, fp1, fp2, "only IP differs — should be masked to same fingerprint")

	fp3 := normalize.Fingerprint("user alice succeeded login from 1.2.3.4")
	assert.NotEqual(t, fp1, fp3)

	fp4 := normalize.Fingerprint("user bob failed login from 1.2.3.4")
	assert.NotEqual(t, fp1, fp4, "name difference is preserved in fingerprint")
}

func TestFingerprint_UUIDMasked(t *testing.T) {
	fp1 := normalize.Fingerprint("request 550e8400-e29b-41d4-a716-446655440000 processed")
	fp2 := normalize.Fingerprint("request 6ba7b810-9dad-11d1-80b4-00c04fd430c8 processed")
	assert.Equal(t, fp1, fp2)
}

func TestFingerprint_Stable(t *testing.T) {
	msg := "failed to open file /var/log/app.log"
	assert.Equal(t, normalize.Fingerprint(msg), normalize.Fingerprint(msg))
}

func TestNormalize_JSONSetsSource(t *testing.T) {
	n := normalize.New()
	r := model.RawLog{
		Raw:        `{"msg":"hello","level":"info"}`,
		Source:     "kafka-prod",
		ReceivedAt: fixedReceivedAt,
	}
	e := n.Normalize(r)
	assert.Equal(t, "kafka-prod", e.Source)
}

func TestNormalize_FallbackTimestamp(t *testing.T) {
	n := normalize.New()
	r := model.RawLog{
		Raw:        `{"msg":"hello"}`,
		Source:     "test",
		ReceivedAt: fixedReceivedAt,
	}
	e := n.Normalize(r)
	assert.Equal(t, fixedReceivedAt.UTC(), e.Timestamp.UTC())
}

func TestNormalize_InternalFieldsDropped(t *testing.T) {
	n := normalize.New()
	r := model.RawLog{
		Raw:        `{"msg":"hello","_internal":"secret","level":"info"}`,
		Source:     "test",
		ReceivedAt: fixedReceivedAt,
	}
	e := n.Normalize(r)
	assert.NotContains(t, e.Fields, "_internal")
}

func TestNormalize_PlainInfersLevel(t *testing.T) {
	n := normalize.New()
	cases := []struct {
		raw  string
		want model.Level
	}{
		{"panic: runtime error", model.LevelFatal},
		{"ERROR something broke", model.LevelError},
		{"connection failed", model.LevelError},
		{"warn: slow query", model.LevelWarn},
		{"starting up", model.LevelInfo},
	}
	for _, tc := range cases {
		e := n.Normalize(model.RawLog{Raw: tc.raw, ReceivedAt: fixedReceivedAt})
		assert.Equal(t, tc.want, e.Level, "input: %q", tc.raw)
	}
}
