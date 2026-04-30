package file_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tragidra/logsense/internal/config"
	"github.com/Tragidra/logsense/internal/ingest/file"
	"github.com/Tragidra/logsense/model"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func collectN(t *testing.T, out <-chan model.RawLog, n int, timeout time.Duration) []model.RawLog {
	t.Helper()
	var got []model.RawLog
	deadline := time.After(timeout)
	for len(got) < n {
		select {
		case r := <-out:
			got = append(got, r)
		case <-deadline:
			t.Fatalf("timeout after %s: collected %d/%d lines", timeout, len(got), n)
		}
	}
	return got
}

func TestNew_MissingFileConfig(t *testing.T) {
	_, err := file.New(&config.SourceConfig{Name: "bad"}, discardLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing [file] config block")
}

func TestNew_MissingPath(t *testing.T) {
	_, err := file.New(&config.SourceConfig{
		Name: "bad",
		File: &config.FileSourceConfig{},
	}, discardLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path is required")
}

func TestStream_EmitsLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	// Create empty file before starting tail.
	f, err := os.Create(path)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	out := make(chan model.RawLog, 20)
	src, err := file.New(&config.SourceConfig{
		Name: "test",
		File: &config.FileSourceConfig{Path: path, StartFrom: "beginning"},
	}, discardLogger())
	require.NoError(t, err)

	go src.Stream(ctx, out) //nolint:errcheck

	time.Sleep(100 * time.Millisecond)

	wantLines := []string{"hello world", "foo bar baz", "error: something failed"}
	for _, l := range wantLines {
		fmt.Fprintln(f, l)
	}
	require.NoError(t, f.Close())

	got := collectN(t, out, len(wantLines), 5*time.Second)

	rawLines := make([]string, len(got))
	for i, r := range got {
		rawLines[i] = r.Raw
	}
	assert.Equal(t, wantLines, rawLines)
}

func TestStream_RawLogFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	f, err := os.Create(path)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	out := make(chan model.RawLog, 5)
	src, err := file.New(&config.SourceConfig{
		Name: "my-source",
		File: &config.FileSourceConfig{Path: path, StartFrom: "beginning"},
	}, discardLogger())
	require.NoError(t, err)

	go src.Stream(ctx, out)
	time.Sleep(100 * time.Millisecond)

	fmt.Fprintln(f, "test line")
	f.Close()

	r := collectN(t, out, 1, 5*time.Second)[0]

	assert.Equal(t, "test line", r.Raw)
	assert.Equal(t, "my-source", r.Source)
	assert.Equal(t, "file", r.SourceKind)
	assert.Equal(t, path, r.Metadata["path"])
	assert.False(t, r.ReceivedAt.IsZero())
}

func TestStream_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	require.NoError(t, os.WriteFile(path, nil, 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan model.RawLog, 5)
	src, err := file.New(&config.SourceConfig{
		Name: "test",
		File: &config.FileSourceConfig{Path: path, StartFrom: "beginning"},
	}, discardLogger())
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() { errCh <- src.Stream(ctx, out) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Stream did not return after context cancel")
	}
}

func TestStream_LongLineTruncated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	f, err := os.Create(path)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	out := make(chan model.RawLog, 5)
	src, err := file.New(&config.SourceConfig{
		Name: "test",
		File: &config.FileSourceConfig{Path: path, StartFrom: "beginning"},
	}, discardLogger())
	require.NoError(t, err)

	go src.Stream(ctx, out) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	longLine := strings.Repeat("a", 100*1024)
	fmt.Fprintln(f, longLine)
	f.Close()

	r := collectN(t, out, 1, 5*time.Second)[0]
	assert.LessOrEqual(t, len(r.Raw), 64*1024)
	assert.Equal(t, "true", r.Metadata["truncated"])
}

func TestStream_FileRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	f, err := os.Create(path)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	out := make(chan model.RawLog, 20)
	src, err := file.New(&config.SourceConfig{
		Name: "test",
		File: &config.FileSourceConfig{Path: path, StartFrom: "beginning"},
	}, discardLogger())
	require.NoError(t, err)

	go src.Stream(ctx, out)
	time.Sleep(100 * time.Millisecond)

	fmt.Fprintln(f, "before rotation")
	require.NoError(t, f.Close())

	pre := collectN(t, out, 1, 5*time.Second)
	assert.Equal(t, "before rotation", pre[0].Raw)

	require.NoError(t, os.Rename(path, path+".1"))
	f2, err := os.Create(path)
	require.NoError(t, err)
	fmt.Fprintln(f2, "after rotation")
	require.NoError(t, f2.Close())

	post := collectN(t, out, 1, 8*time.Second)
	assert.Equal(t, "after rotation", post[0].Raw)
}
