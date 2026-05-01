// Package file implements a tail-based log ingest source.
package file

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/nxadm/tail"

	"github.com/Tragidra/logstruct/internal/config"
	"github.com/Tragidra/logstruct/internal/ingest"
	"github.com/Tragidra/logstruct/model"
)

const maxLineBytes = 64 * 1024

// FileSource tails a file and emits one RawLog per line.
type FileSource struct {
	name   string
	cfg    *config.FileSourceConfig
	logger *slog.Logger
}

// New validates the config and returns a ready FileSource.
func New(cfg *config.SourceConfig, logger *slog.Logger) (ingest.Source, error) {
	if cfg.File == nil {
		return nil, fmt.Errorf("file source %q: missing [file] config block", cfg.Name)
	}
	if cfg.File.Path == "" {
		return nil, fmt.Errorf("file source %q: path is required", cfg.Name)
	}
	return &FileSource{name: cfg.Name, cfg: cfg.File, logger: logger}, nil
}

func (s *FileSource) Name() string { return s.name }

// Stream tails the configured file and sends one RawLog per line to out.
func (s *FileSource) Stream(ctx context.Context, out chan<- model.RawLog) error {
	seekInfo := &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd}
	if s.cfg.StartFrom == "beginning" {
		seekInfo = &tail.SeekInfo{Offset: 0, Whence: io.SeekStart}
	}

	t, err := tail.TailFile(s.cfg.Path, tail.Config{
		Follow:    true,
		ReOpen:    true,
		MustExist: false,
		Location:  seekInfo,
		Logger:    tail.DiscardingLogger,
	})
	if err != nil {
		return fmt.Errorf("file source %q: tail: %w", s.name, err)
	}
	defer t.Cleanup()

	for {
		select {
		case <-ctx.Done():
			t.Stop()
			return nil
		case line, ok := <-t.Lines:
			if !ok {
				return t.Err()
			}
			if line.Err != nil {
				s.logger.Warn("file source: line error",
					"source", s.name, "path", s.cfg.Path, "err", line.Err)
				continue
			}
			raw, truncated := processLine(line.Text)
			meta := map[string]string{"path": s.cfg.Path}
			if truncated {
				meta["truncated"] = "true"
			}
			select {
			case out <- model.RawLog{
				Raw:        raw,
				Source:     s.name,
				SourceKind: "file",
				ReceivedAt: line.Time,
				Metadata:   meta,
			}:
			case <-ctx.Done():
				t.Stop()
				return nil
			}
		}
	}
}

// processLine replaces non-UTF-8 bytes with U+FFFD and truncates to 64 KB.
func processLine(s string) (out string, truncated bool) {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\ufffd")
	}
	if len(s) <= maxLineBytes {
		return s, false
	}
	// Walk back from the limit to find a rune boundary.
	i := maxLineBytes
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return s[:i], true
}
